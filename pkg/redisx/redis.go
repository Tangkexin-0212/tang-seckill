// Package redisx 封装 go-redis 客户端与所有 Lua 脚本调用。
package redisx

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"seckill/pkg/config"
	"seckill/pkg/lua"
)

// Client 带脚本缓存的 Redis 客户端。
type Client struct {
	rdb     *redis.Client
	scripts map[string]*redis.Script
}

var allScripts = []string{
	lua.TokenBucket,
	lua.Seckill,
	lua.Release,
	lua.CartAdd,
	lua.CartSeckillAdd,
	lua.CartSet,
}

// New 创建客户端并预加载脚本（首次 Run 时才真正 EVALSHA）。
func New(conf config.RedisConf) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         conf.Addr,
		Password:     conf.Password,
		DB:           conf.DB,
		PoolSize:     conf.PoolSize,
		MinIdleConns: conf.MinIdleConns,
		DialTimeout:  time.Duration(conf.DialTimeoutMs) * time.Millisecond,
		ReadTimeout:  time.Duration(conf.ReadTimeoutMs) * time.Millisecond,
		WriteTimeout: time.Duration(conf.WriteTimeoutMs) * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("ping redis %s: %w", conf.Addr, err)
	}

	c := &Client{rdb: rdb, scripts: make(map[string]*redis.Script, len(allScripts))}
	for _, name := range allScripts {
		c.scripts[name] = redis.NewScript(lua.Script(name))
	}
	return c, nil
}

// Raw 返回底层客户端，供普通读写命令使用。
func (c *Client) Raw() *redis.Client { return c.rdb }

// Close 关闭连接。
func (c *Client) Close() error { return c.rdb.Close() }

func (c *Client) run(ctx context.Context, name string, keys []string, args ...interface{}) ([]int64, error) {
	script, ok := c.scripts[name]
	if !ok {
		return nil, fmt.Errorf("lua script %s not loaded", name)
	}
	raw, err := script.Run(ctx, c.rdb, keys, args...).Result()
	if err != nil {
		return nil, err
	}
	return toInt64Slice(raw), nil
}

// ------------------------------------------------------------
// 令牌桶限流
// ------------------------------------------------------------

// Allow 执行一次令牌桶判定，返回是否放行与剩余令牌。
func (c *Client) Allow(ctx context.Context, bucketKey string, capacity, ratePerSecond float64, requested int64) (bool, int64, error) {
	res, err := c.run(ctx, lua.TokenBucket,
		[]string{bucketKey},
		formatFloat(capacity), formatFloat(ratePerSecond), strconv.FormatInt(time.Now().UnixMilli(), 10), requested,
	)
	if err != nil {
		return false, 0, err
	}
	if len(res) < 2 {
		return false, 0, fmt.Errorf("unexpected token bucket reply: %v", res)
	}
	return res[0] == 1, res[1], nil
}

// ------------------------------------------------------------
// 秒杀 / 释放
// ------------------------------------------------------------

// 秒杀 Lua 返回码
const (
	SeckillOK            int64 = 1
	SeckillAlreadyBought int64 = -1
	SeckillSoldOut       int64 = -2
	SeckillNotWarmed     int64 = -3
)

// HasSeckillUser 只读判断用户是否已抢到该活动的资格（O(1)）。
// 供幂等前置检查使用：让"已抢过"的重复请求在限流之前就能被正确应答。
func (c *Client) HasSeckillUser(ctx context.Context, usersKey string, uid int64) (bool, error) {
	return c.rdb.SIsMember(ctx, usersKey, uid).Result()
}

// DoSeckill 原子执行"判重 + 扣库存"，返回 Lua 原始码与扣减后的剩余库存。
func (c *Client) DoSeckill(ctx context.Context, stockKey, usersKey string, uid int64) (int64, int32, error) {
	res, err := c.run(ctx, lua.Seckill, []string{stockKey, usersKey}, uid)
	if err != nil {
		return 0, 0, err
	}
	if len(res) < 2 {
		return 0, 0, fmt.Errorf("unexpected seckill reply: %v", res)
	}
	return res[0], int32(res[1]), nil
}

// DoRelease 原子执行"摘除延时任务 + 移除幂等标记 + 回补库存"，返回 Lua 原始码。
// orderNo 传空字符串时跳过延时队列判定（用于订单尚未创建的回滚场景）。
func (c *Client) DoRelease(ctx context.Context, stockKey, usersKey, delayKey, orderNo string, uid int64) (int64, error) {
	res, err := c.run(ctx, lua.Release, []string{stockKey, usersKey, delayKey}, uid, orderNo)
	if err != nil {
		return 0, err
	}
	if len(res) < 1 {
		return 0, fmt.Errorf("unexpected release reply: %v", res)
	}
	return res[0], nil
}

// ------------------------------------------------------------
// 购物车
// ------------------------------------------------------------

// 购物车 Lua 返回码
const (
	CartOK        int64 = 1
	CartExisted   int64 = 0
	CartDeleted   int64 = 2
	CartOverLimit int64 = -1
	CartBadArg    int64 = -2
)

// CartAdd 普通加购。
func (c *Client) CartAdd(ctx context.Context, cartKey string, productID int64, delta, maxPerItem int32) (int64, int32, error) {
	res, err := c.run(ctx, lua.CartAdd, []string{cartKey}, productID, delta, maxPerItem)
	if err != nil {
		return 0, 0, err
	}
	return pair(res)
}

// CartSeckillAdd 秒杀加购（幂等，qty 恒为 1）。
func (c *Client) CartSeckillAdd(ctx context.Context, cartKey string, productID int64, qty int32) (int64, int32, error) {
	res, err := c.run(ctx, lua.CartSeckillAdd, []string{cartKey}, productID, qty)
	if err != nil {
		return 0, 0, err
	}
	return pair(res)
}

// CartSet 直接设置数量。
func (c *Client) CartSet(ctx context.Context, cartKey string, productID int64, qty, maxPerItem int32) (int64, int32, error) {
	res, err := c.run(ctx, lua.CartSet, []string{cartKey}, productID, qty, maxPerItem)
	if err != nil {
		return 0, 0, err
	}
	return pair(res)
}

// ------------------------------------------------------------
// 内部工具
// ------------------------------------------------------------

func pair(res []int64) (int64, int32, error) {
	if len(res) < 2 {
		return 0, 0, fmt.Errorf("unexpected cart reply: %v", res)
	}
	return res[0], int32(res[1]), nil
}

func toInt64Slice(raw interface{}) []int64 {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]int64, 0, len(arr))
	for _, item := range arr {
		out = append(out, toInt64(item))
	}
	return out
}

func toInt64(v interface{}) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	case string:
		n, err := strconv.ParseInt(t, 10, 64)
		if err != nil {
			f, ferr := strconv.ParseFloat(t, 64)
			if ferr != nil {
				return 0
			}
			return int64(f)
		}
		return n
	default:
		return 0
	}
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
