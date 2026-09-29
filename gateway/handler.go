package main

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/pkg/config"
	"seckill/pkg/rpcx"
	"seckill/pkg/utils"
)

// Gateway 聚合所有下游服务客户端，对外提供 HTTP 接口。
type Gateway struct {
	cfg *config.Config
	cli *rpcx.Clients
}

// NewGateway 构造网关。
func NewGateway(cfg *config.Config, cli *rpcx.Clients) *Gateway {
	return &Gateway{cfg: cfg, cli: cli}
}

// ------------------------------------------------------------
// 用户
// ------------------------------------------------------------

// Register 注册。
func (g *Gateway) Register(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !bindJSON(c, &req) {
		return
	}

	uid, err := g.cli.User.Register(ctx, req.Username, req.Password)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, map[string]interface{}{"uid": uid})
}

// Login 登录，返回 token。
func (g *Gateway) Login(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !bindJSON(c, &req) {
		return
	}

	resp, err := g.cli.User.Login(ctx, req.Username, req.Password)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// ------------------------------------------------------------
// 秒杀
// ------------------------------------------------------------

// GetSeckillInfo 活动详情（纯 Redis 读，压测时不会给 MySQL 造成压力）。
func (g *Gateway) GetSeckillInfo(ctx context.Context, c *app.RequestContext) {
	activityID := parseInt64(c.Query("activityId"))
	if activityID <= 0 {
		badRequest(c, "activityId 无效")
		return
	}

	info, err := g.cli.Seckill.GetSeckillInfo(ctx, activityID)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, info)
}

// DoSeckill 秒杀主接口。userId 完全来自网关鉴权结果，不接受前端传入。
func (g *Gateway) DoSeckill(ctx context.Context, c *app.RequestContext) {
	uid := currentUID(c)

	var req struct {
		ActivityId int64 `json:"activityId"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.ActivityId <= 0 {
		badRequest(c, "activityId 无效")
		return
	}

	resp, err := g.cli.Seckill.Seckill(ctx, uid, req.ActivityId)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, resp)
}

// CancelSeckill 主动取消，用于演示库存释放。
func (g *Gateway) CancelSeckill(ctx context.Context, c *app.RequestContext) {
	uid := currentUID(c)

	var req struct {
		ActivityId int64 `json:"activityId"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.ActivityId <= 0 {
		badRequest(c, "activityId 无效")
		return
	}

	released, err := g.cli.Seckill.CancelSeckill(ctx, uid, req.ActivityId)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, map[string]interface{}{"released": released})
}

// ------------------------------------------------------------
// 购物车
// ------------------------------------------------------------

// ListCart 购物车列表。
func (g *Gateway) ListCart(ctx context.Context, c *app.RequestContext) {
	items, err := g.cli.Cart.ListCart(ctx, currentUID(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, items)
}

// CountCart 购物车条目数。
// 走 HLen，只需要一个整数，不必把整个购物车（两个 Hash 全量）拉回网关——
// 顶栏角标、侧栏徽标这类场景只关心数量。
func (g *Gateway) CountCart(ctx context.Context, c *app.RequestContext) {
	n, err := g.cli.Cart.CountItems(ctx, currentUID(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, map[string]interface{}{"count": n})
}

// AddCartItem 普通加购。
func (g *Gateway) AddCartItem(ctx context.Context, c *app.RequestContext) {
	var req struct {
		ProductId int64 `json:"productId"`
		Qty       int32 `json:"qty"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.ProductId <= 0 || req.Qty <= 0 {
		badRequest(c, "productId/qty 无效")
		return
	}

	item, err := g.cli.Cart.AddItem(ctx, currentUID(c), req.ProductId, req.Qty)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, item)
}

// UpdateCartItem 修改数量，qty<=0 删除。
func (g *Gateway) UpdateCartItem(ctx context.Context, c *app.RequestContext) {
	var req struct {
		ProductId int64 `json:"productId"`
		Qty       int32 `json:"qty"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.ProductId <= 0 {
		badRequest(c, "productId 无效")
		return
	}

	item, err := g.cli.Cart.UpdateQty(ctx, currentUID(c), req.ProductId, req.Qty)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, item)
}

// RemoveCartItem 删除条目。
func (g *Gateway) RemoveCartItem(ctx context.Context, c *app.RequestContext) {
	var req struct {
		ProductId int64 `json:"productId"`
	}
	if !bindJSON(c, &req) {
		return
	}

	removed, err := g.cli.Cart.RemoveItem(ctx, currentUID(c), req.ProductId)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, map[string]interface{}{"removed": removed})
}

// ClearCart 清空购物车。
func (g *Gateway) ClearCart(ctx context.Context, c *app.RequestContext) {
	cleared, err := g.cli.Cart.ClearCart(ctx, currentUID(c))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, map[string]interface{}{"cleared": cleared})
}

// ------------------------------------------------------------
// 订单
// ------------------------------------------------------------

// PayOrder 模拟支付。
func (g *Gateway) PayOrder(ctx context.Context, c *app.RequestContext) {
	var req struct {
		OrderNo string `json:"orderNo"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.OrderNo == "" {
		badRequest(c, "orderNo 不能为空")
		return
	}

	ord, err := g.cli.Order.Pay(ctx, req.OrderNo)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, ord)
}

// ListOrders 我的订单。
func (g *Gateway) ListOrders(ctx context.Context, c *app.RequestContext) {
	limit := int32(parseInt64(c.Query("limit")))
	if limit <= 0 {
		limit = 20
	}

	list, err := g.cli.Order.ListOrders(ctx, currentUID(c), limit)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// GetOrder 订单详情。
func (g *Gateway) GetOrder(ctx context.Context, c *app.RequestContext) {
	orderNo := c.Query("orderNo")
	if orderNo == "" {
		badRequest(c, "orderNo 不能为空")
		return
	}

	ord, err := g.cli.Order.GetOrder(ctx, orderNo)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, ord)
}

// ------------------------------------------------------------
// 商品与活动（管理侧，供压测准备数据）
// ------------------------------------------------------------

// ListProducts 商品列表。
func (g *Gateway) ListProducts(ctx context.Context, c *app.RequestContext) {
	limit := int32(parseInt64(c.Query("limit")))
	offset := int32(parseInt64(c.Query("offset")))
	if limit <= 0 {
		limit = 20
	}

	list, err := g.cli.Product.ListProducts(ctx, limit, offset)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// ListActivities 活动列表。
func (g *Gateway) ListActivities(ctx context.Context, c *app.RequestContext) {
	limit := int32(parseInt64(c.Query("limit")))
	if limit <= 0 {
		limit = 20
	}

	list, err := g.cli.Product.ListActivities(ctx, limit)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, list)
}

// CreateProduct 创建商品。
func (g *Gateway) CreateProduct(ctx context.Context, c *app.RequestContext) {
	var req struct {
		Name        string `json:"name"`
		Price       int64  `json:"price"`
		Cover       string `json:"cover"`
		Description string `json:"description"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.Name == "" || req.Price <= 0 {
		badRequest(c, "name/price 无效")
		return
	}

	id, err := g.cli.Product.CreateProduct(ctx, req.Name, req.Price, req.Cover, req.Description)
	if err != nil {
		fail(c, err)
		return
	}

	// 顺手初始化库存，避免压测前还要多调一次接口
	if _, serr := g.cli.Stock.InitStock(ctx, id, 0); serr != nil {
		klog.Warnf("init stock for product %d failed: %v", id, serr)
	}
	ok(c, map[string]interface{}{"productId": id})
}

// CreateActivity 创建秒杀活动。timeoutSeconds 为 0 时默认 10 分钟后结束。
func (g *Gateway) CreateActivity(ctx context.Context, c *app.RequestContext) {
	var req struct {
		ProductId    int64 `json:"productId"`
		SeckillPrice int64 `json:"seckillPrice"`
		TotalStock   int32 `json:"totalStock"`
		StartTimeMs  int64 `json:"startTimeMs"`
		EndTimeMs    int64 `json:"endTimeMs"`
		TimeoutSec   int64 `json:"timeoutSeconds"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.ProductId <= 0 || req.TotalStock <= 0 {
		badRequest(c, "productId/totalStock 无效")
		return
	}

	now := utils.NowMs()
	if req.StartTimeMs <= 0 {
		req.StartTimeMs = now
	}
	if req.EndTimeMs <= 0 {
		timeout := req.TimeoutSec
		if timeout <= 0 {
			timeout = 600
		}
		req.EndTimeMs = now + timeout*1000
	}

	id, err := g.cli.Product.CreateActivity(ctx, req.ProductId, req.SeckillPrice,
		req.TotalStock, req.StartTimeMs, req.EndTimeMs)
	if err != nil {
		fail(c, err)
		return
	}

	// 活动库存同步进 stock_db，用于最终一致性对账
	if _, serr := g.cli.Stock.InitStock(ctx, req.ProductId, req.TotalStock); serr != nil {
		klog.Warnf("init stock of product %d to %d failed: %v", req.ProductId, req.TotalStock, serr)
	}
	ok(c, map[string]interface{}{"activityId": id})
}

// WarmUp 手动预热活动到 Redis（等价于 cron 的一次同步）。
func (g *Gateway) WarmUp(ctx context.Context, c *app.RequestContext) {
	var req struct {
		ActivityId int64 `json:"activityId"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if req.ActivityId <= 0 {
		badRequest(c, "activityId 无效")
		return
	}

	warmed, err := g.cli.Seckill.WarmUp(ctx, req.ActivityId)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, map[string]interface{}{"warmed": warmed})
}

// ------------------------------------------------------------
// 工具
// ------------------------------------------------------------

func bindJSON(c *app.RequestContext, dst interface{}) bool {
	body, err := c.Body()
	if err != nil || len(body) == 0 {
		badRequest(c, "请求体为空")
		return false
	}
	if err = json.Unmarshal(body, dst); err != nil {
		badRequest(c, "请求体 JSON 解析失败")
		return false
	}
	return true
}
