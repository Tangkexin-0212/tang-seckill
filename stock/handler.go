// 库存服务：MySQL 侧库存与库存流水（秒杀链路的最终一致兜底）。
package main

import (
	"context"
	"errors"

	"github.com/cloudwego/kitex/pkg/klog"
	"gorm.io/gorm"

	"seckill/kitex_gen/stock"
	"seckill/pkg/errno"
	"seckill/pkg/models"
	"seckill/pkg/mysqlx"
)

// 事务内部哨兵错误
var (
	errFlowDuplicated = errors.New("stock flow duplicated")
	errNotEnoughStock = errors.New("not enough stock")
)

// Handler 库存服务实现。
type Handler struct {
	db *gorm.DB
}

// NewHandler 构造库存服务处理器。
func NewHandler(db *gorm.DB) *Handler { return &Handler{db: db} }

// GetStock 查询库存。
func (h *Handler) GetStock(ctx context.Context, productID int64) (*stock.StockInfo, error) {
	var s models.Stock
	if err := h.db.WithContext(ctx).Where("product_id = ?", uint64(productID)).First(&s).Error; err != nil {
		if mysqlx.IsRecordNotFound(err) {
			return nil, errno.ToBizError(errno.WithMsg(errno.BadRequest, "库存记录不存在"))
		}
		return nil, errno.ToBizError(errno.Internal)
	}
	return &stock.StockInfo{ProductId: int64(s.ProductID), Total: s.Total, Available: s.Available}, nil
}

// InitStock 初始化（或重置）库存。
func (h *Handler) InitStock(ctx context.Context, productID int64, total int32) (bool, error) {
	if productID <= 0 || total < 0 {
		return false, errno.ToBizError(errno.BadRequest)
	}
	var s models.Stock
	err := h.db.WithContext(ctx).Where("product_id = ?", uint64(productID)).First(&s).Error
	if err != nil && !mysqlx.IsRecordNotFound(err) {
		return false, errno.ToBizError(errno.Internal)
	}
	if mysqlx.IsRecordNotFound(err) {
		return true, h.db.WithContext(ctx).Create(&models.Stock{
			ProductID: uint64(productID), Total: total, Available: total,
		}).Error
	}
	s.Total, s.Available = total, total
	if err = h.db.WithContext(ctx).Model(&s).Updates(map[string]interface{}{
		"total": total, "available": total,
	}).Error; err != nil {
		return false, errno.ToBizError(errno.Internal)
	}
	return true, nil
}

// Deduct DB 侧扣减库存，orderNo 保证幂等。
func (h *Handler) Deduct(ctx context.Context, activityID, userID, productID int64, orderNo string) (bool, error) {
	return h.applyFlow(ctx, activityID, userID, productID, orderNo, models.StockFlowDeduct)
}

// Rollback DB 侧回补库存。
func (h *Handler) Rollback(ctx context.Context, activityID, userID, productID int64, orderNo string) (bool, error) {
	return h.applyFlow(ctx, activityID, userID, productID, orderNo, models.StockFlowRollback)
}

// applyFlow 在一个事务内"写流水 + 改库存"。
// uk(order_no, type) 唯一索引保证同一订单的同类型操作只会生效一次。
func (h *Handler) applyFlow(ctx context.Context, activityID, userID, productID int64, orderNo string, flowType int8) (bool, error) {
	if orderNo == "" || productID <= 0 {
		return false, errno.ToBizError(errno.BadRequest)
	}

	delta := int32(-1)
	if flowType == models.StockFlowRollback {
		delta = 1
	}

	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		flow := &models.StockFlow{
			ActivityID: uint64(activityID),
			UserID:     uint64(userID),
			OrderNo:    orderNo,
			Delta:      delta,
			Type:       flowType,
		}
		if err := tx.Create(flow).Error; err != nil {
			if mysqlx.IsDupErr(err) {
				return errFlowDuplicated
			}
			return err
		}

		q := tx.Model(&models.Stock{}).Where("product_id = ?", uint64(productID))
		if flowType == models.StockFlowDeduct {
			// 条件更新，杜绝负库存
			q = q.Where("available >= 1")
		}
		res := q.UpdateColumn("available", gorm.Expr("available + ?", delta))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errNotEnoughStock
		}
		return nil
	})

	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, errFlowDuplicated):
		// 幂等命中：同一订单已处理过
		return true, nil
	case errors.Is(err, errNotEnoughStock):
		return false, errno.ToBizError(errno.SoldOut)
	default:
		klog.Errorf("apply stock flow failed orderNo=%s type=%d: %v", orderNo, flowType, err)
		return false, errno.ToBizError(errno.Internal)
	}
}

// DeductedCount 返回该活动在 DB 中的净扣减数量（扣减 - 回补），用于对账。
func (h *Handler) DeductedCount(ctx context.Context, activityID int64) (int32, error) {
	type row struct {
		Type  int8
		Total int32
	}
	var rows []row
	if err := h.db.WithContext(ctx).Model(&models.StockFlow{}).
		Select("type, count(*) as total").
		Where("activity_id = ?", uint64(activityID)).
		Group("type").
		Scan(&rows).Error; err != nil {
		klog.Errorf("count stock flow failed activity=%d: %v", activityID, err)
		return 0, errno.ToBizError(errno.Internal)
	}

	var net int32
	for _, r := range rows {
		if r.Type == models.StockFlowDeduct {
			net += r.Total
		} else {
			net -= r.Total
		}
	}
	return net, nil
}
