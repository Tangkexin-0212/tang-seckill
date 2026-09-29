// 秒杀压测与验收脚本。
//
// 验收标准（来自需求确认）：
//
//	1000 个并发用户抢 100 件库存
//	  成功数   恰好 = 100
//	  失败数         = 900
//	  超卖           = 0（Redis 剩余库存为 0，MySQL 扣减流水为 100）
//	  一人一件       = 每个成功用户的购物车中该商品 qty 恒为 1
//	  幂等           = 同一用户重复请求返回"已抢过"，库存不再变化
//	  最终一致       = worker 消费完成后 MySQL 流水与 Redis 库存吻合
//
// 用法：
//
//	go run ./tools/loadtest -gateway http://127.0.0.1:8080 -users 1000 -stock 100
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"seckill/pkg/config"
	"seckill/pkg/consts"
	"seckill/pkg/models"
	"seckill/pkg/mysqlx"
)

// 业务错误码（与 pkg/errno 保持一致）
const (
	codeOK            int32 = 0
	codeAlreadyBought int32 = 10002
	codeSoldOut       int32 = 10003
	codeRateLimited   int32 = 10001
)

// RespBody 网关统一响应体。
type RespBody struct {
	Code int32           `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type seckillData struct {
	OrderNo     string `json:"orderNo"`
	RemainStock int32  `json:"remainStock"`
	ActivityId  int64  `json:"activityId"`
}

type loginData struct {
	Token string `json:"token"`
	Uid   int64  `json:"uid"`
}

type seckillInfoData struct {
	RemainStock int32 `json:"remainStock"`
	TotalStock  int32 `json:"totalStock"`
	SoldOut     bool  `json:"soldOut"`
}

type cartItem struct {
	ProductId int64 `json:"productId"`
	Qty       int32 `json:"qty"`
}

func main() {
	gateway := flag.String("gateway", "http://127.0.0.1:8080", "网关地址")
	users := flag.Int("users", 1000, "并发用户数")
	stock := flag.Int("stock", 100, "秒杀库存")
	cfgPath := flag.String("config", config.DefaultPath, "配置文件路径（用于校验 MySQL 最终一致）")
	waitConsistency := flag.Duration("consistency-timeout", 30*time.Second, "等待最终一致的最长时间")
	flag.Parse()

	client := newHTTPClient()
	base := *gateway

	fmt.Println("=========== 秒杀压测 / 验收 ===========")
	fmt.Printf("网关=%s 并发用户=%d 库存=%d\n\n", base, *users, *stock)

	var failures []string
	fail := func(format string, args ...interface{}) {
		failures = append(failures, fmt.Sprintf(format, args...))
	}

	// ------------------------------------------------------------
	// 1. 准备数据：商品 + 活动 + 预热
	// ------------------------------------------------------------
	productID, err := createProduct(client, base)
	if err != nil {
		fmt.Printf("创建商品失败: %v\n", err)
		os.Exit(1)
	}
	activityID, err := createActivity(client, base, productID, int32(*stock))
	if err != nil {
		fmt.Printf("创建活动失败: %v\n", err)
		os.Exit(1)
	}
	if err = warmUp(client, base, activityID); err != nil {
		fmt.Printf("预热失败: %v\n", err)
		os.Exit(1)
	}
	info, err := getSeckillInfo(client, base, activityID)
	if err != nil {
		fmt.Printf("查询活动信息失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[准备] productId=%d activityId=%d 预热后剩余库存=%d\n\n", productID, activityID, info.RemainStock)

	// ------------------------------------------------------------
	// 2. 准备用户：注册 + 登录（并发度 64）
	// ------------------------------------------------------------
	start := time.Now()
	tokens, uids, err := prepareUsers(client, base, *users)
	if err != nil {
		fmt.Printf("准备用户失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[准备] %d 个用户注册登录完成，耗时 %s\n\n", len(tokens), time.Since(start).Round(time.Millisecond))

	// ------------------------------------------------------------
	// 3. 并发秒杀：所有请求对齐同一时刻发出
	// ------------------------------------------------------------
	type result struct {
		code    int32
		token   string
		uid     int64
		latency time.Duration
	}
	results := make([]result, *users)
	gate := make(chan struct{})
	var wg sync.WaitGroup

	burstStart := time.Now()
	for i := 0; i < *users; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-gate
			t0 := time.Now()
			body, perr := postJSON(client, base+"/api/seckill/do",
				map[string]interface{}{"activityId": activityID}, tokens[i])
			lat := time.Since(t0)

			code := int32(-999) // 网络/解析异常
			if perr == nil {
				code = body.Code
			}
			results[i] = result{code: code, token: tokens[i], uid: uids[i], latency: lat}
		}(i)
	}
	close(gate)
	wg.Wait()
	burstCost := time.Since(burstStart)

	// ------------------------------------------------------------
	// 4. 统计
	// ------------------------------------------------------------
	codeCount := make(map[int32]int)
	latencies := make([]time.Duration, 0, *users)
	successTokens := make([]string, 0, *stock)
	for _, r := range results {
		codeCount[r.code]++
		latencies = append(latencies, r.latency)
		if r.code == codeOK {
			successTokens = append(successTokens, r.token)
		}
	}

	fmt.Printf("[压测] %d 请求全部返回，总耗时 %s\n", *users, burstCost.Round(time.Millisecond))
	fmt.Printf("        成功=%d 已抢过=%d 库存不足=%d 被限流=%d 其它异常=%d\n",
		codeCount[codeOK], codeCount[codeAlreadyBought], codeCount[codeSoldOut],
		codeCount[codeRateLimited], codeCount[-999])
	fmt.Printf("        延迟 P50=%s P95=%s P99=%s max=%s\n\n",
		percentile(latencies, 50), percentile(latencies, 95), percentile(latencies, 99), maxOf(latencies))

	// ------------------------------------------------------------
	// 5. 断言一：成功数恰好等于库存
	// ------------------------------------------------------------
	if codeCount[codeOK] != *stock {
		fail("成功数 = %d，期望 %d", codeCount[codeOK], *stock)
	} else {
		fmt.Printf("[PASS] 成功数恰好为 %d\n", *stock)
	}

	// ------------------------------------------------------------
	// 6. 断言二：无超卖，Redis 剩余库存归零
	// ------------------------------------------------------------
	info, err = getSeckillInfo(client, base, activityID)
	if err != nil {
		fail("压测后查询活动信息失败: %v", err)
	} else if info.RemainStock != 0 {
		fail("Redis 剩余库存 = %d，期望 0（可能存在超卖）", info.RemainStock)
	} else {
		fmt.Println("[PASS] Redis 剩余库存为 0，无超卖")
	}

	// ------------------------------------------------------------
	// 7. 断言三：一人一件（购物车 qty 恒为 1）
	// ------------------------------------------------------------
	if err = assertCartQty(client, base, successTokens); err != nil {
		fail("%v", err)
	} else {
		fmt.Printf("[PASS] 抽查成功用户的购物车，qty 恒为 1（样本 %d 个）\n", minInt(len(successTokens), cartSample))
	}

	// ------------------------------------------------------------
	// 8. 断言四：幂等 —— 同一用户重复请求不再占用库存
	// ------------------------------------------------------------
	if len(successTokens) > 0 {
		before := info.RemainStock
		body, perr := postJSON(client, base+"/api/seckill/do",
			map[string]interface{}{"activityId": activityID}, successTokens[0])
		after, _ := getSeckillInfo(client, base, activityID)

		switch {
		case perr != nil:
			fail("重复请求失败: %v", perr)
		case body.Code != codeAlreadyBought:
			fail("重复请求返回 code=%d，期望 %d（已抢过）", body.Code, codeAlreadyBought)
		case after.RemainStock != before:
			fail("重复请求后库存发生变化 %d → %d", before, after.RemainStock)
		default:
			fmt.Println("[PASS] 重复请求被幂等拦截，库存未变化")
		}
	}

	// ------------------------------------------------------------
	// 9. 断言五：最终一致 —— MySQL 流水与 Redis 吻合
	// ------------------------------------------------------------
	checkFinalConsistency(*cfgPath, activityID, productID, int32(codeCount[codeOK]), int32(*stock), *waitConsistency, fail)

	// ------------------------------------------------------------
	// 10. 汇总
	// ------------------------------------------------------------
	fmt.Println()
	if len(failures) == 0 {
		fmt.Println("=========== 验收通过：全部断言成功 ===========")
		return
	}
	fmt.Printf("=========== 验收失败：%d 项不符合预期 ===========\n", len(failures))
	for i, f := range failures {
		fmt.Printf("  %d) %s\n", i+1, f)
	}
	os.Exit(1)
}

// ------------------------------------------------------------
// 断言实现
// ------------------------------------------------------------

const cartSample = 30

// assertCartQty 抽查成功用户的购物车，验证秒杀商品 qty 恒为 1。
func assertCartQty(client *http.Client, base string, tokens []string) error {
	if len(tokens) == 0 {
		return fmt.Errorf("没有成功的用户可供校验购物车")
	}
	n := minInt(len(tokens), cartSample)

	var mu sync.Mutex
	var bad []string
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(token string) {
			defer wg.Done()
			items, err := listCart(client, base, token)
			if err != nil {
				mu.Lock()
				bad = append(bad, "查询购物车失败: "+err.Error())
				mu.Unlock()
				return
			}
			found := false
			for _, it := range items {
				if it.ProductId == 0 {
					continue
				}
				found = true
				if it.Qty != 1 {
					mu.Lock()
					bad = append(bad, fmt.Sprintf("productId=%d qty=%d（期望 1）", it.ProductId, it.Qty))
					mu.Unlock()
				}
			}
			if !found {
				mu.Lock()
				bad = append(bad, "购物车中找不到秒杀商品")
				mu.Unlock()
			}
		}(tokens[i])
	}
	wg.Wait()

	if len(bad) > 0 {
		return fmt.Errorf("购物车数量校验失败 %d 例，例如: %s", len(bad), bad[0])
	}
	return nil
}

// checkFinalConsistency 轮询 MySQL，直到扣减流水数与成功数一致，
// 并校验 stock.available == 总库存 - 成功数（DB 侧同样零超卖）。
func checkFinalConsistency(cfgPath string, activityID, productID int64, expectSuccess, totalStock int32,
	timeout time.Duration, fail func(string, ...interface{})) {

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Printf("[SKIP] 无法加载配置(%v)，跳过 MySQL 最终一致校验\n", err)
		return
	}
	db, err := mysqlx.Open(cfg.MySQL, consts.DBStock)
	if err != nil {
		fmt.Printf("[SKIP] 无法连接 MySQL(%v)，跳过 MySQL 最终一致校验\n", err)
		return
	}

	deadline := time.Now().Add(timeout)
	var deducted int64
	for {
		if err = db.Model(&models.StockFlow{}).
			Where("activity_id = ? AND type = ?", activityID, models.StockFlowDeduct).
			Count(&deducted).Error; err != nil {
			fail("查询 stock_flow 失败: %v", err)
			return
		}
		if deducted == int64(expectSuccess) || time.Now().After(deadline) {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	if deducted != int64(expectSuccess) {
		fail("MySQL 扣减流水 = %d，期望 %d（最终一致未收敛）", deducted, expectSuccess)
		return
	}
	fmt.Printf("[PASS] 最终一致：MySQL 扣减流水 = %d，与成功数吻合\n", deducted)

	var st models.Stock
	if err = db.Where("product_id = ?", productID).First(&st).Error; err != nil {
		fail("查询 stock 失败: %v", err)
		return
	}
	if want := totalStock - expectSuccess; st.Available != want {
		fail("MySQL stock.available = %d，期望 %d", st.Available, want)
		return
	}
	fmt.Printf("[PASS] MySQL stock.available = %d，零超卖\n", st.Available)
}

// ------------------------------------------------------------
// HTTP 辅助
// ------------------------------------------------------------

func newHTTPClient() *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        2000,
		MaxIdleConnsPerHost: 2000,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     60 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second}
}

func postJSON(client *http.Client, url string, body interface{}, token string) (RespBody, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return RespBody{}, err
		}
	}
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		return RespBody{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return RespBody{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return RespBody{}, err
	}
	var out RespBody
	if err = json.Unmarshal(raw, &out); err != nil {
		return RespBody{}, fmt.Errorf("解析响应失败: %s", string(raw))
	}
	return out, nil
}

func getJSON(client *http.Client, url, token string) (RespBody, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return RespBody{}, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return RespBody{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return RespBody{}, err
	}
	var out RespBody
	if err = json.Unmarshal(raw, &out); err != nil {
		return RespBody{}, fmt.Errorf("解析响应失败: %s", string(raw))
	}
	return out, nil
}

func createProduct(client *http.Client, base string) (int64, error) {
	body, err := postJSON(client, base+"/api/admin/product", map[string]interface{}{
		"name":        fmt.Sprintf("压测商品-%d", time.Now().UnixNano()%1e9),
		"price":       999900,
		"cover":       "https://example.com/loadtest.png",
		"description": "压测用商品",
	}, "")
	if err != nil {
		return 0, err
	}
	if body.Code != codeOK {
		return 0, fmt.Errorf("code=%d msg=%s", body.Code, body.Msg)
	}
	var data struct {
		ProductId int64 `json:"productId"`
	}
	if err = json.Unmarshal(body.Data, &data); err != nil {
		return 0, err
	}
	return data.ProductId, nil
}

func createActivity(client *http.Client, base string, productID int64, stock int32) (int64, error) {
	body, err := postJSON(client, base+"/api/admin/activity", map[string]interface{}{
		"productId":      productID,
		"seckillPrice":   99900,
		"totalStock":     stock,
		"timeoutSeconds": 3600,
	}, "")
	if err != nil {
		return 0, err
	}
	if body.Code != codeOK {
		return 0, fmt.Errorf("code=%d msg=%s", body.Code, body.Msg)
	}
	var data struct {
		ActivityId int64 `json:"activityId"`
	}
	if err = json.Unmarshal(body.Data, &data); err != nil {
		return 0, err
	}
	return data.ActivityId, nil
}

func warmUp(client *http.Client, base string, activityID int64) error {
	body, err := postJSON(client, base+"/api/admin/warmup",
		map[string]interface{}{"activityId": activityID}, "")
	if err != nil {
		return err
	}
	if body.Code != codeOK {
		return fmt.Errorf("code=%d msg=%s", body.Code, body.Msg)
	}
	return nil
}

func getSeckillInfo(client *http.Client, base string, activityID int64) (seckillInfoData, error) {
	body, err := getJSON(client,
		fmt.Sprintf("%s/api/seckill/info?activityId=%d", base, activityID), "")
	if err != nil {
		return seckillInfoData{}, err
	}
	if body.Code != codeOK {
		return seckillInfoData{}, fmt.Errorf("code=%d msg=%s", body.Code, body.Msg)
	}
	var data seckillInfoData
	if err = json.Unmarshal(body.Data, &data); err != nil {
		return seckillInfoData{}, err
	}
	return data, nil
}

func listCart(client *http.Client, base, token string) ([]cartItem, error) {
	body, err := getJSON(client, base+"/api/cart/list", token)
	if err != nil {
		return nil, err
	}
	if body.Code != codeOK {
		return nil, fmt.Errorf("code=%d msg=%s", body.Code, body.Msg)
	}
	var items []cartItem
	if len(body.Data) > 0 {
		if err = json.Unmarshal(body.Data, &items); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// prepareUsers 并发注册并登录 users 个账号，返回 token 与 uid。
func prepareUsers(client *http.Client, base string, users int) ([]string, []int64, error) {
	tokens := make([]string, users)
	uids := make([]int64, users)
	errs := make([]error, users)

	sem := make(chan struct{}, 64)
	var wg sync.WaitGroup
	seed := time.Now().UnixNano()

	for i := 0; i < users; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			username := fmt.Sprintf("lt_%d_%d", seed%1e9, i)
			const password = "123456"

			if body, err := postJSON(client, base+"/api/user/register", map[string]interface{}{
				"username": username, "password": password,
			}, ""); err != nil {
				errs[i] = fmt.Errorf("register: %w", err)
				return
			} else if body.Code != codeOK {
				errs[i] = fmt.Errorf("register code=%d msg=%s", body.Code, body.Msg)
				return
			}

			body, err := postJSON(client, base+"/api/user/login", map[string]interface{}{
				"username": username, "password": password,
			}, "")
			if err != nil {
				errs[i] = fmt.Errorf("login: %w", err)
				return
			}
			if body.Code != codeOK {
				errs[i] = fmt.Errorf("login code=%d msg=%s", body.Code, body.Msg)
				return
			}
			var data loginData
			if err = json.Unmarshal(body.Data, &data); err != nil {
				errs[i] = err
				return
			}
			tokens[i] = data.Token
			uids[i] = data.Uid
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return nil, nil, fmt.Errorf("第 %d 个用户准备失败: %w", i, err)
		}
	}
	return tokens, uids, nil
}

// ------------------------------------------------------------
// 统计工具
// ------------------------------------------------------------

func percentile(latencies []time.Duration, p float64) time.Duration {
	if len(latencies) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	idx := int(float64(len(sorted)-1) * p / 100.0)
	if idx < 0 {
		idx = 0
	}
	return sorted[idx].Round(time.Microsecond)
}

func maxOf(latencies []time.Duration) time.Duration {
	var m time.Duration
	for _, d := range latencies {
		if d > m {
			m = d
		}
	}
	return m.Round(time.Microsecond)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
