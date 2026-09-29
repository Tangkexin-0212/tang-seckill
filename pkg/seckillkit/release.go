package seckillkit

import (
	"context"
	"fmt"

	"seckill/kitex_gen/cart/cartservice"
	"seckill/kitex_gen/order"
	"seckill/kitex_gen/order/orderservice"
	"seckill/pkg/config"
	"seckill/pkg/consts"
	"seckill/pkg/models"
	"seckill/pkg/redisx"
)

// ReleaseOutcome 释放结果。
type ReleaseOutcome int

// 释放结果枚举
const (
	// ReleaseNone 无需释放（订单已支付/已取消，或无秒杀资格标记）
	ReleaseNone ReleaseOutcome = iota
	// ReleaseRestored 已成功回补 Redis 库存
	ReleaseRestored
	// ReleaseActivityEnded 活动已结束，仅清理了幂等标记
	ReleaseActivityEnded
)

func (o ReleaseOutcome) String() string {
	switch o {
	case ReleaseRestored:
		return "restored"
	case ReleaseActivityEnded:
		return "activity_ended"
	default:
		return "none"
	}
}

// Releaser 负责把"秒杀资格"归还给用户：
// 回补 Redis 库存 + 删除幂等标记 + 取消订单 + 移除购物车条目。
// cron/worker（超时释放）与 seckill（主动取消）共用同一实现。
type Releaser struct {
	Redis *redisx.Client
	Cfg   *config.Config
	Order orderservice.Client
	Cart  cartservice.Client
}

// NewReleaser 构造释放器。
func NewReleaser(rdb *redisx.Client, cfg *config.Config, oc orderservice.Client, cc cartservice.Client) *Releaser {
	return &Releaser{Redis: rdb, Cfg: cfg, Order: oc, Cart: cc}
}

// ReleaseByOrderNo 按订单号释放。
func (r *Releaser) ReleaseByOrderNo(ctx context.Context, orderNo, reason string) (ReleaseOutcome, error) {
	ord, err := r.Order.GetOrder(ctx, orderNo)
	if err != nil {
		return ReleaseNone, fmt.Errorf("get order %s: %w", orderNo, err)
	}
	if ord.Status != models.OrderPending {
		// 已支付或已取消，无需释放
		return ReleaseNone, nil
	}

	code, err := r.Redis.DoRelease(ctx,
		consts.SeckillStockKey(ord.ActivityId),
		consts.SeckillUsersKey(ord.ActivityId),
		consts.OrderDelayZSet,
		orderNo, ord.UserId)
	if err != nil {
		return ReleaseNone, fmt.Errorf("run release lua for order %s: %w", orderNo, err)
	}

	switch code {
	case 1:
		r.afterRelease(ctx, ord, reason)
		return ReleaseRestored, nil
	case -1:
		r.afterRelease(ctx, ord, reason)
		return ReleaseActivityEnded, nil
	default:
		// Lua 返回 0：延时任务已被摘除（通常是"刚支付成功"的竞态），或用户已不持有资格标记。
		// 复查订单状态，避免出现永远悬挂的 pending 订单。此处不回补库存。
		fresh, gerr := r.Order.GetOrder(ctx, orderNo)
		if gerr == nil && fresh.Status == models.OrderPending {
			if _, cerr := r.Order.Cancel(ctx, orderNo, reason); cerr != nil {
				LogWarn("cancel stale pending order %s failed: %v", orderNo, cerr)
			} else {
				LogWarn("order %s released without stock restore (eligibility mark missing)", orderNo)
			}
		}
		return ReleaseNone, nil
	}
}

// RollbackWithoutOrder 订单尚未创建时的回滚：
// 归还库存与幂等标记；此时延时队列里没有该订单，购物车条目由调用方自行清理。
func (r *Releaser) RollbackWithoutOrder(ctx context.Context, userID, activityID int64) (ReleaseOutcome, error) {
	code, err := r.Redis.DoRelease(ctx,
		consts.SeckillStockKey(activityID),
		consts.SeckillUsersKey(activityID),
		consts.OrderDelayZSet,
		"", userID)
	if err != nil {
		return ReleaseNone, fmt.Errorf("rollback uid=%d act=%d: %w", userID, activityID, err)
	}
	switch code {
	case 1:
		return ReleaseRestored, nil
	case -1:
		return ReleaseActivityEnded, nil
	default:
		return ReleaseNone, nil
	}
}

// ReleaseByUserActivity 按 (用户, 活动) 释放，用于主动取消场景。
func (r *Releaser) ReleaseByUserActivity(ctx context.Context, userID, activityID int64, reason string) (ReleaseOutcome, error) {
	ord, err := r.Order.GetOrderByUserActivity(ctx, userID, activityID)
	if err != nil {
		return ReleaseNone, fmt.Errorf("get order of uid=%d act=%d: %w", userID, activityID, err)
	}
	return r.ReleaseByOrderNo(ctx, ord.OrderNo, reason)
}

func (r *Releaser) afterRelease(ctx context.Context, ord *order.Order, reason string) {
	if _, err := r.Order.Cancel(ctx, ord.OrderNo, reason); err != nil {
		LogWarn("cancel order %s failed: %v", ord.OrderNo, err)
	}
	if _, err := r.Cart.RemoveItem(ctx, ord.UserId, ord.ProductId); err != nil {
		LogWarn("remove cart item of order %s failed: %v", ord.OrderNo, err)
	}
}
