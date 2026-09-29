// 秒杀编排服务：整个抢购链路的大脑。
//
// 一次成功的抢购 = 限流 → 原子扣 Redis 库存 → 幂等加购物车 → 创建待支付订单
// → 投递落库事件（异步最终一致）。任何一步失败都会把库存与幂等标记回滚，
// 保证"一个人绝不会占到多个名额"且"库存不会被凭空吞掉"。
package main

import (
	"context"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/kitex_gen/cart/cartservice"
	"seckill/kitex_gen/order"
	"seckill/kitex_gen/order/orderservice"
	"seckill/kitex_gen/product"
	"seckill/kitex_gen/product/productservice"
	"seckill/kitex_gen/seckill"
	"seckill/pkg/config"
	"seckill/pkg/consts"
	"seckill/pkg/errno"
	"seckill/pkg/redisx"
	"seckill/pkg/seckillkit"
	"seckill/pkg/utils"
)

// Handler 秒杀服务实现。
type Handler struct {
	cfg     *config.Config
	rdb     *redisx.Client
	sync    *seckillkit.Syncer
	rel     *seckillkit.Releaser
	lim     *Limiter
	product productservice.Client
	cart    cartservice.Client
	order   orderservice.Client
}

// NewHandler 构造秒杀处理器。
func NewHandler(
	cfg *config.Config,
	rdb *redisx.Client,
	sync *seckillkit.Syncer,
	rel *seckillkit.Releaser,
	lim *Limiter,
	product productservice.Client,
	cart cartservice.Client,
	order orderservice.Client,
) *Handler {
	return &Handler{
		cfg: cfg, rdb: rdb, sync: sync, rel: rel, lim: lim,
		product: product, cart: cart, order: order,
	}
}

// Seckill 秒杀主接口。
//
// 返回的 *seckill.SeckillResp 只在"抢到"时返回，失败一律通过业务错误返回错误码，
// 这样网关侧无需解析 body 里的 code 字段。
func (h *Handler) Seckill(ctx context.Context, userID, activityID int64) (*seckill.SeckillResp, error) {
	if userID <= 0 || activityID <= 0 {
		return nil, errno.ToBizError(errno.BadRequest)
	}

	// 1) 活动快照：优先复用中间件/上游已读到的结果，避免同一次请求重复打 Redis
	snap, remain, ok := seckillkit.SnapshotFrom(ctx)
	if !ok {
		snap, remain = h.sync.LoadOrNil(ctx, activityID)
	}
	if snap == nil {
		// 未预热（活动不存在、或已结束被 TTL 清理、或 cron 还没跑）
		klog.Warnf("seckill rejected: activity %d not warmed up, uid=%d", activityID, userID)
		return nil, errno.ToBizError(errno.ActivityInvalid)
	}
	if !snap.InWindow(time.Now()) {
		return nil, errno.ToBizError(errno.ActivityInvalid)
	}

	// 2) 幂等前置判定：已抢过的用户直接返回原订单号。
	// 必须放在限流之前：
	//   - 活动令牌桶容量只有库存的 1.5 倍，售罄后桶已见底，若先限流，
	//     重复请求会拿到 10001/10003，而不是语义上正确的 10002；
	//   - "已抢过"是只读的 O(1) 判定，不需要消耗宝贵的限流令牌。
	// 这里不做提前返回"库存不足"：remain 是并发前读到的脏值，
	// 真实的是否售罄由 seckill.lua 原子判定（脚本内判重先于判库存）。
	if bought, err := h.rdb.HasSeckillUser(ctx, consts.SeckillUsersKey(activityID), userID); err != nil {
		// Redis 抖动时放行，交给后面的 Lua 做权威判定
		klog.Warnf("idempotency precheck failed uid=%d act=%d: %v", userID, activityID, err)
	} else if bought {
		return h.alreadyBought(ctx, userID, activityID)
	}

	// 3) 三层令牌桶限流，挡住脚本刷单与瞬时洪峰
	if e := h.lim.Allow(ctx, userID, activityID, snap, remain); e != nil {
		return nil, errno.ToBizError(e)
	}

	// 4) 原子扣减：判重 + 扣库存 + 记录资格，三步在同一个 Lua 里完成
	code, remainAfter, err := h.rdb.DoSeckill(ctx,
		consts.SeckillStockKey(activityID), consts.SeckillUsersKey(activityID), userID)
	if err != nil {
		klog.Errorf("seckill lua failed uid=%d act=%d: %v", userID, activityID, err)
		return nil, errno.ToBizError(errno.Internal)
	}

	switch code {
	case redisx.SeckillAlreadyBought:
		// 幂等语义：重复请求不报"系统错误"，而是明确告知已参与并返回原订单号
		return h.alreadyBought(ctx, userID, activityID)
	case redisx.SeckillSoldOut:
		return nil, errno.ToBizError(errno.SoldOut)
	case redisx.SeckillNotWarmed:
		return nil, errno.ToBizError(errno.ActivityInvalid)
	case redisx.SeckillOK:
		// 继续走后续链路
	default:
		klog.Errorf("unexpected seckill code=%d uid=%d act=%d", code, userID, activityID)
		return nil, errno.ToBizError(errno.Internal)
	}

	orderNo, e := h.fulfill(ctx, userID, activityID, snap)
	if e != nil {
		return nil, errno.ToBizError(e)
	}

	klog.Infof("seckill success uid=%d act=%d orderNo=%s remain=%d", userID, activityID, orderNo, remainAfter)
	return &seckill.SeckillResp{
		Code:        errno.CodeOK,
		Msg:         "抢购成功",
		OrderNo:     orderNo,
		RemainStock: remainAfter,
		ActivityId:  activityID,
		UserId:      userID,
	}, nil
}

// fulfill 扣减成功后的收尾：加购物车 → 建订单 → 投递落库事件。
// 任一步失败都会回滚库存与幂等标记，让用户可以重新抢。
func (h *Handler) fulfill(ctx context.Context, userID, activityID int64, snap *seckillkit.Snapshot) (string, *errno.Err) {
	// 3.1 幂等加购：Lua 用 HSETNX，重复调用 qty 恒为 1，不会累加成 2、3、4
	if _, err := h.cart.SeckillAdd(ctx, userID, snap.ProductID, activityID,
		snap.SeckillPrice, snap.ProductName, snap.Cover); err != nil {
		klog.Errorf("cart seckill add failed uid=%d act=%d: %v", userID, activityID, err)
		h.rollback(ctx, userID, activityID, "cart add failed")
		return "", errno.From(err)
	}

	// 3.2 创建待支付订单：order 服务用 uk(user_id, activity_id) 做 DB 侧幂等，
	//     并在新建成功时把订单放进 ZSet 延时队列
	ttl := int64(h.cfg.Order.PayTimeoutSeconds)
	ord, err := h.order.CreatePending(ctx, &order.CreatePendingReq{
		UserId:      userID,
		ActivityId:  activityID,
		ProductId:   snap.ProductID,
		ProductName: snap.ProductName,
		Amount:      snap.SeckillPrice,
		TtlSeconds:  ttl,
	})
	if err != nil || ord == nil {
		// ord 为 nil 说明下游返回了空响应：同样必须回滚，否则库存会被凭空吞掉
		klog.Errorf("create pending order failed uid=%d act=%d: err=%v ordNil=%v",
			userID, activityID, err, ord == nil)
		h.rollback(ctx, userID, activityID, "create order failed")
		if _, rerr := h.cart.RemoveItem(ctx, userID, snap.ProductID); rerr != nil {
			klog.Warnf("remove cart item after rollback failed uid=%d pid=%d: %v",
				userID, snap.ProductID, rerr)
		}
		if err == nil {
			return "", errno.WithMsg(errno.Internal, "订单创建返回空")
		}
		return "", errno.From(err)
	}

	// 3.3 投递落库事件：Redis 是权威库存，MySQL 由 worker 异步补齐（最终一致）。
	//     这里刻意不因为投递失败而回滚 —— 抢购已经成功，事件丢失由对账任务兜底。
	if err := seckillkit.PublishEvent(ctx, h.rdb, &seckillkit.Event{
		Type:        consts.EventSeckillSuccess,
		OrderNo:     ord.OrderNo,
		UserID:      userID,
		ActivityID:  activityID,
		ProductID:   snap.ProductID,
		Amount:      snap.SeckillPrice,
		CreatedAtMs: utils.NowMs(),
	}); err != nil {
		klog.Errorf("publish seckill event failed orderNo=%s: %v", ord.OrderNo, err)
	}

	return ord.OrderNo, nil
}

// rollback 归还库存与幂等标记；此时订单尚未创建，延时队列里没有该订单。
func (h *Handler) rollback(ctx context.Context, userID, activityID int64, reason string) {
	out, err := h.rel.RollbackWithoutOrder(ctx, userID, activityID)
	if err != nil {
		// 回滚失败必须告警：需要人工或对账任务修复
		klog.Errorf("rollback failed uid=%d act=%d reason=%s: %v", userID, activityID, reason, err)
		return
	}
	klog.Warnf("rollback done uid=%d act=%d reason=%s outcome=%s", userID, activityID, reason, out)
}

// alreadyBought 幂等命中：返回用户在该活动中已持有的订单号。
func (h *Handler) alreadyBought(ctx context.Context, userID, activityID int64) (*seckill.SeckillResp, error) {
	ord, err := h.order.GetOrderByUserActivity(ctx, userID, activityID)
	if err != nil || ord == nil || ord.OrderNo == "" {
		// 拿不到订单（例如订单已被清理）时也要给出明确语义
		return nil, errno.ToBizError(errno.AlreadyBought)
	}
	return nil, errno.ToBizError(errno.WithMsg(errno.AlreadyBought,
		"您已参与过该活动，订单号 "+ord.OrderNo))
}

// GetSeckillInfo 活动页信息：只读 Redis 快照，未预热时降级回源商品服务。
func (h *Handler) GetSeckillInfo(ctx context.Context, activityID int64) (*seckill.SeckillInfo, error) {
	if activityID <= 0 {
		return nil, errno.ToBizError(errno.BadRequest)
	}

	snap, remain := h.sync.LoadOrNil(ctx, activityID)
	if snap == nil {
		return h.infoFromMySQL(ctx, activityID)
	}

	soldOut := remain <= 0
	return &seckill.SeckillInfo{
		ActivityId:    snap.ActivityID,
		ProductId:     snap.ProductID,
		ProductName:   snap.ProductName,
		Cover:         snap.Cover,
		OriginalPrice: snap.OriginalPrice,
		SeckillPrice:  snap.SeckillPrice,
		TotalStock:    snap.TotalStock,
		RemainStock:   remain,
		StartTimeMs:   snap.StartTimeMs,
		EndTimeMs:     snap.EndTimeMs,
		Status:        snap.Status,
		SoldOut:       soldOut,
	}, nil
}

// infoFromMySQL Redis 未预热时的降级路径：回源商品服务，并顺手预热。
func (h *Handler) infoFromMySQL(ctx context.Context, activityID int64) (*seckill.SeckillInfo, error) {
	act, err := h.product.GetActivity(ctx, activityID)
	if err != nil {
		return nil, errno.From(err)
	}

	remain := act.Stock
	if remain <= 0 {
		remain = act.TotalStock
	}
	// best-effort 预热，失败不影响本次查询
	if _, werr := h.WarmUp(ctx, activityID); werr != nil {
		klog.Warnf("warm up activity %d on demand failed: %v", activityID, werr)
	}

	return &seckill.SeckillInfo{
		ActivityId:    act.Id,
		ProductId:     act.ProductId,
		ProductName:   act.ProductName,
		Cover:         act.Cover,
		OriginalPrice: act.OriginalPrice,
		SeckillPrice:  act.SeckillPrice,
		TotalStock:    act.TotalStock,
		RemainStock:   remain,
		StartTimeMs:   act.StartTimeMs,
		EndTimeMs:     act.EndTimeMs,
		Status:        act.Status,
		SoldOut:       remain <= 0,
	}, nil
}

// CancelSeckill 主动取消：回补库存、删除幂等标记、取消订单、移除购物车条目。
func (h *Handler) CancelSeckill(ctx context.Context, userID, activityID int64) (bool, error) {
	if userID <= 0 || activityID <= 0 {
		return false, errno.ToBizError(errno.BadRequest)
	}
	out, err := h.rel.ReleaseByUserActivity(ctx, userID, activityID, "user cancel")
	if err != nil {
		return false, errno.ToBizError(errno.From(err))
	}
	return out == seckillkit.ReleaseRestored, nil
}

// WarmUp 手动预热单个活动，复用 cron 的同步实现，避免两套逻辑漂移。
func (h *Handler) WarmUp(ctx context.Context, activityID int64) (bool, error) {
	if activityID <= 0 {
		return false, errno.ToBizError(errno.BadRequest)
	}
	act, err := h.product.GetActivity(ctx, activityID)
	if err != nil {
		return false, errno.From(err)
	}

	nowMs := utils.NowMs()
	if act.EndTimeMs <= nowMs {
		// 已结束：清理 Redis 残留
		if err = h.sync.Remove(ctx, activityID); err != nil {
			return false, errno.ToBizError(errno.Internal)
		}
		return false, nil
	}

	if err = h.sync.Sync(ctx, &product.SeckillSyncItem{
		ActivityId:    act.Id,
		ProductId:     act.ProductId,
		ProductName:   act.ProductName,
		Cover:         act.Cover,
		OriginalPrice: act.OriginalPrice,
		SeckillPrice:  act.SeckillPrice,
		TotalStock:    act.TotalStock,
		Stock:         act.Stock,
		StartTimeMs:   act.StartTimeMs,
		EndTimeMs:     act.EndTimeMs,
		Status:        act.Status,
	}); err != nil {
		return false, errno.ToBizError(errno.Internal)
	}
	return true, nil
}
