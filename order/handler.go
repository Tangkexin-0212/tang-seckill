// 订单服务：待支付订单的创建、支付、取消，并维护 ZSet 延时队列。
package main

import (
	"context"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"seckill/kitex_gen/order"
	"seckill/pkg/config"
	"seckill/pkg/consts"
	"seckill/pkg/errno"
	"seckill/pkg/models"
	"seckill/pkg/mysqlx"
	"seckill/pkg/redisx"
	"seckill/pkg/utils"
)

// Handler 订单服务实现。
type Handler struct {
	db  *gorm.DB
	rdb *redisx.Client
	cfg *config.Config
}

// NewHandler 构造订单服务处理器。
func NewHandler(db *gorm.DB, rdb *redisx.Client, cfg *config.Config) *Handler {
	return &Handler{db: db, rdb: rdb, cfg: cfg}
}

// CreatePending 幂等创建待支付订单，并把订单放入延时队列。
func (h *Handler) CreatePending(ctx context.Context, req *order.CreatePendingReq) (*order.Order, error) {
	if req == nil || req.UserId <= 0 || req.ActivityId <= 0 || req.ProductId <= 0 {
		return nil, errno.ToBizError(errno.BadRequest)
	}

	ttl := req.TtlSeconds
	if ttl <= 0 {
		ttl = int64(h.cfg.Order.PayTimeoutSeconds)
	}

	expireAt := time.Now().Add(time.Duration(ttl) * time.Second)
	o := &models.Order{
		OrderNo:     utils.NewOrderNo(),
		UserID:      uint64(req.UserId),
		ActivityID:  uint64(req.ActivityId),
		ProductID:   uint64(req.ProductId),
		ProductName: req.ProductName,
		Amount:      req.Amount,
		Status:      models.OrderPending,
		ExpireAt:    expireAt,
	}

	if err := h.db.WithContext(ctx).Create(o).Error; err != nil {
		if !mysqlx.IsDupErr(err) {
			klog.Errorf("create order failed uid=%d act=%d: %v", req.UserId, req.ActivityId, err)
			return nil, errno.ToBizError(errno.Internal)
		}
		// uk(user_id, activity_id) 命中 → 幂等返回已有订单
		var exist models.Order
		if err = h.db.WithContext(ctx).
			Where("user_id = ? AND activity_id = ?", uint64(req.UserId), uint64(req.ActivityId)).
			First(&exist).Error; err != nil {
			return nil, errno.ToBizError(errno.Internal)
		}
		o = &exist
		klog.Infof("order already exists, idempotent hit: uid=%d act=%d orderNo=%s",
			req.UserId, req.ActivityId, o.OrderNo)
	} else {
		// 只有新建成功才需要入延时队列（重复订单不入队，避免重复释放）
		if err = h.rdb.Raw().ZAdd(ctx, consts.OrderDelayZSet, redisZ(expireAt, o.OrderNo)).Err(); err != nil {
			klog.Errorf("zadd delay queue failed orderNo=%s: %v", o.OrderNo, err)
		}
	}

	return toOrder(o), nil
}

// Pay 模拟支付：pending -> paid。已支付时幂等返回，已取消时返回状态错误。
func (h *Handler) Pay(ctx context.Context, orderNo string) (*order.Order, error) {
	if orderNo == "" {
		return nil, errno.ToBizError(errno.BadRequest)
	}

	now := time.Now()
	res := h.db.WithContext(ctx).Model(&models.Order{}).
		Where("order_no = ? AND status = ?", orderNo, models.OrderPending).
		Updates(map[string]interface{}{"status": models.OrderPaid, "paid_at": now})
	if res.Error != nil {
		klog.Errorf("pay order %s failed: %v", orderNo, res.Error)
		return nil, errno.ToBizError(errno.Internal)
	}

	// 支付成功：把订单从延时队列摘除，超时任务因此不会再回滚它
	if res.RowsAffected > 0 {
		pipe := h.rdb.Raw().Pipeline()
		pipe.ZRem(ctx, consts.OrderDelayZSet, orderNo)
		pipe.SAdd(ctx, consts.OrderPaidSet, orderNo)
		pipe.Expire(ctx, consts.OrderPaidSet, 24*time.Hour)
		if _, err := pipe.Exec(ctx); err != nil {
			klog.Errorf("remove order %s from delay queue failed: %v", orderNo, err)
		}
	}

	o, err := h.getOrderModel(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if o.Status == models.OrderCancelled {
		return nil, errno.ToBizError(errno.WithMsg(errno.OrderState, "订单已取消，无法支付"))
	}
	return toOrder(o), nil
}

// Cancel 取消订单（仅 pending 可取消），并摘除延时队列。
func (h *Handler) Cancel(ctx context.Context, orderNo, reason string) (bool, error) {
	if orderNo == "" {
		return false, errno.ToBizError(errno.BadRequest)
	}

	res := h.db.WithContext(ctx).Model(&models.Order{}).
		Where("order_no = ? AND status = ?", orderNo, models.OrderPending).
		Update("status", models.OrderCancelled)
	if res.Error != nil {
		klog.Errorf("cancel order %s failed: %v", orderNo, res.Error)
		return false, errno.ToBizError(errno.Internal)
	}

	if res.RowsAffected > 0 {
		if err := h.rdb.Raw().ZRem(ctx, consts.OrderDelayZSet, orderNo).Err(); err != nil {
			klog.Warnf("zrem order %s failed: %v", orderNo, err)
		}
		return true, nil
	}

	// 已经是取消状态视为幂等成功；已支付则不允许取消
	o, err := h.getOrderModel(ctx, orderNo)
	if err != nil {
		return false, err
	}
	if o.Status == models.OrderCancelled {
		return true, nil
	}
	return false, errno.ToBizError(errno.WithMsg(errno.OrderState, "已支付订单无法取消"))
}

// GetOrder 查询订单。
func (h *Handler) GetOrder(ctx context.Context, orderNo string) (*order.Order, error) {
	o, err := h.getOrderModel(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	return toOrder(o), nil
}

// GetOrderByUserActivity 按 (用户, 活动) 查订单。
func (h *Handler) GetOrderByUserActivity(ctx context.Context, userID, activityID int64) (*order.Order, error) {
	var o models.Order
	if err := h.db.WithContext(ctx).
		Where("user_id = ? AND activity_id = ?", uint64(userID), uint64(activityID)).
		First(&o).Error; err != nil {
		if mysqlx.IsRecordNotFound(err) {
			return nil, errno.ToBizError(errno.OrderNotFound)
		}
		return nil, errno.ToBizError(errno.Internal)
	}
	return toOrder(&o), nil
}

// ListOrders 查询用户订单。
func (h *Handler) ListOrders(ctx context.Context, userID int64, limit int32) ([]*order.Order, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var list []models.Order
	if err := h.db.WithContext(ctx).
		Where("user_id = ?", uint64(userID)).
		Order("id desc").Limit(int(limit)).
		Find(&list).Error; err != nil {
		return nil, errno.ToBizError(errno.Internal)
	}
	out := make([]*order.Order, 0, len(list))
	for i := range list {
		out = append(out, toOrder(&list[i]))
	}
	return out, nil
}

// ------------------------------------------------------------
// 内部方法
// ------------------------------------------------------------

func (h *Handler) getOrderModel(ctx context.Context, orderNo string) (*models.Order, error) {
	var o models.Order
	if err := h.db.WithContext(ctx).Where("order_no = ?", orderNo).First(&o).Error; err != nil {
		if mysqlx.IsRecordNotFound(err) {
			return nil, errno.ToBizError(errno.OrderNotFound)
		}
		return nil, errno.ToBizError(errno.Internal)
	}
	return &o, nil
}

func toOrder(o *models.Order) *order.Order {
	return &order.Order{
		OrderNo:     o.OrderNo,
		UserId:      int64(o.UserID),
		ActivityId:  int64(o.ActivityID),
		ProductId:   int64(o.ProductID),
		ProductName: o.ProductName,
		Amount:      o.Amount,
		Status:      o.Status,
		ExpireAtMs:  utils.ToMs(o.ExpireAt),
		CreatedAtMs: utils.ToMs(o.CreatedAt),
		PaidAtMs:    utils.ToMsPtr(o.PaidAt),
	}
}

func redisZ(t time.Time, member string) redis.Z {
	return redis.Z{Score: float64(t.UnixMilli()), Member: member}
}
