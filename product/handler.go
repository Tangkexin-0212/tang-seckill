// 商品与秒杀活动服务。
package main

import (
	"context"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"gorm.io/gorm"

	"seckill/kitex_gen/product"
	"seckill/pkg/errno"
	"seckill/pkg/models"
	"seckill/pkg/mysqlx"
	"seckill/pkg/utils"
)

// Handler 商品服务实现。
type Handler struct {
	db *gorm.DB
}

// NewHandler 构造商品服务处理器。
func NewHandler(db *gorm.DB) *Handler { return &Handler{db: db} }

// CreateProduct 新建商品。
func (h *Handler) CreateProduct(ctx context.Context, name string, price int64, cover, description string) (int64, error) {
	if name == "" || price <= 0 {
		return 0, errno.ToBizError(errno.BadRequest)
	}
	p := &models.Product{Name: name, Price: price, Cover: cover, Description: description, Status: 1}
	if err := h.db.WithContext(ctx).Create(p).Error; err != nil {
		klog.Errorf("create product failed: %v", err)
		return 0, errno.ToBizError(errno.Internal)
	}
	return int64(p.ID), nil
}

// GetProduct 查询商品。
func (h *Handler) GetProduct(ctx context.Context, productID int64) (*product.Product, error) {
	var p models.Product
	if err := h.db.WithContext(ctx).First(&p, uint64(productID)).Error; err != nil {
		if mysqlx.IsRecordNotFound(err) {
			return nil, errno.ToBizError(errno.WithMsg(errno.BadRequest, "商品不存在"))
		}
		return nil, errno.ToBizError(errno.Internal)
	}
	return toProduct(&p), nil
}

// ListProducts 分页查询商品。
func (h *Handler) ListProducts(ctx context.Context, limit, offset int32) ([]*product.Product, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var list []models.Product
	if err := h.db.WithContext(ctx).
		Where("status = 1").
		Order("id desc").Limit(int(limit)).Offset(int(offset)).
		Find(&list).Error; err != nil {
		return nil, errno.ToBizError(errno.Internal)
	}
	out := make([]*product.Product, 0, len(list))
	for i := range list {
		out = append(out, toProduct(&list[i]))
	}
	return out, nil
}

// CreateActivity 新建秒杀活动。
func (h *Handler) CreateActivity(ctx context.Context, productID, seckillPrice int64, totalStock int32, startMs, endMs int64) (int64, error) {
	if productID <= 0 || totalStock <= 0 || startMs <= 0 || endMs <= startMs {
		return 0, errno.ToBizError(errno.BadRequest)
	}
	act := &models.SeckillActivity{
		ProductID:    uint64(productID),
		SeckillPrice: seckillPrice,
		TotalStock:   totalStock,
		Stock:        totalStock,
		StartTime:    utils.FromMs(startMs),
		EndTime:      utils.FromMs(endMs),
		Status:       models.ActivityNotStarted,
	}
	now := utils.NowMs()
	if now >= startMs && now < endMs {
		act.Status = models.ActivityRunning
	}
	if err := h.db.WithContext(ctx).Create(act).Error; err != nil {
		klog.Errorf("create activity failed: %v", err)
		return 0, errno.ToBizError(errno.Internal)
	}
	return int64(act.ID), nil
}

// GetActivity 查询活动（含商品信息）。
func (h *Handler) GetActivity(ctx context.Context, activityID int64) (*product.SeckillActivity, error) {
	var act models.SeckillActivity
	if err := h.db.WithContext(ctx).First(&act, uint64(activityID)).Error; err != nil {
		if mysqlx.IsRecordNotFound(err) {
			return nil, errno.ToBizError(errno.WithMsg(errno.ActivityInvalid, "活动不存在"))
		}
		return nil, errno.ToBizError(errno.Internal)
	}

	var p models.Product
	if err := h.db.WithContext(ctx).First(&p, act.ProductID).Error; err != nil {
		return nil, errno.ToBizError(errno.Internal)
	}
	return toActivity(&act, &p), nil
}

// ListActivities 查询活动列表。
func (h *Handler) ListActivities(ctx context.Context, limit int32) ([]*product.SeckillActivity, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var acts []models.SeckillActivity
	if err := h.db.WithContext(ctx).Order("id desc").Limit(int(limit)).Find(&acts).Error; err != nil {
		return nil, errno.ToBizError(errno.Internal)
	}
	out := make([]*product.SeckillActivity, 0, len(acts))
	for i := range acts {
		var p models.Product
		if err := h.db.WithContext(ctx).First(&p, acts[i].ProductID).Error; err != nil {
			continue
		}
		out = append(out, toActivity(&acts[i], &p))
	}
	return out, nil
}

// ListSyncTargets 返回需要同步到 Redis 的活动（未结束的活动）。
// cron 会周期性调用它，把结果写入 Redis。
func (h *Handler) ListSyncTargets(ctx context.Context, nowMs int64) ([]*product.SeckillSyncItem, error) {
	now := utils.FromMs(nowMs)
	if nowMs <= 0 {
		now = time.Now()
	}

	var acts []models.SeckillActivity
	if err := h.db.WithContext(ctx).
		Where("status <> ? AND end_time > ?", models.ActivityFinished, now).
		Order("id asc").
		Find(&acts).Error; err != nil {
		klog.Errorf("list sync targets failed: %v", err)
		return nil, errno.ToBizError(errno.Internal)
	}

	items := make([]*product.SeckillSyncItem, 0, len(acts))
	for i := range acts {
		var p models.Product
		if err := h.db.WithContext(ctx).First(&p, acts[i].ProductID).Error; err != nil {
			continue
		}
		status := acts[i].Status
		if now.Before(acts[i].StartTime) {
			status = models.ActivityNotStarted
		} else if now.Before(acts[i].EndTime) {
			status = models.ActivityRunning
		} else {
			status = models.ActivityFinished
		}
		items = append(items, &product.SeckillSyncItem{
			ActivityId:    int64(acts[i].ID),
			ProductId:     int64(acts[i].ProductID),
			ProductName:   p.Name,
			Cover:         p.Cover,
			OriginalPrice: p.Price,
			SeckillPrice:  acts[i].SeckillPrice,
			TotalStock:    acts[i].TotalStock,
			Stock:         acts[i].Stock,
			StartTimeMs:   utils.ToMs(acts[i].StartTime),
			EndTimeMs:     utils.ToMs(acts[i].EndTime),
			Status:        status,
		})
	}
	return items, nil
}

// FinishActivity 结束活动。
func (h *Handler) FinishActivity(ctx context.Context, activityID int64) (bool, error) {
	return h.UpdateActivityStatus(ctx, activityID, models.ActivityFinished)
}

// UpdateActivityStatus 更新活动状态。
func (h *Handler) UpdateActivityStatus(ctx context.Context, activityID int64, status int32) (bool, error) {
	res := h.db.WithContext(ctx).Model(&models.SeckillActivity{}).
		Where("id = ?", uint64(activityID)).
		Update("status", status)
	if res.Error != nil {
		klog.Errorf("update activity %d status failed: %v", activityID, res.Error)
		return false, errno.ToBizError(errno.Internal)
	}
	return res.RowsAffected > 0, nil
}

// ------------------------------------------------------------
// 转换
// ------------------------------------------------------------

func toProduct(p *models.Product) *product.Product {
	return &product.Product{
		Id:          int64(p.ID),
		Name:        p.Name,
		Price:       p.Price,
		Cover:       p.Cover,
		Description: p.Description,
		Status:      p.Status,
	}
}

func toActivity(a *models.SeckillActivity, p *models.Product) *product.SeckillActivity {
	return &product.SeckillActivity{
		Id:            int64(a.ID),
		ProductId:     int64(a.ProductID),
		SeckillPrice:  a.SeckillPrice,
		TotalStock:    a.TotalStock,
		Stock:         a.Stock,
		StartTimeMs:   utils.ToMs(a.StartTime),
		EndTimeMs:     utils.ToMs(a.EndTime),
		Status:        a.Status,
		ProductName:   p.Name,
		Cover:         p.Cover,
		OriginalPrice: p.Price,
	}
}
