// Package seckillkit 收敛秒杀活动的 Redis 读写逻辑。
// cron（定时同步）、seckill（秒杀编排）、worker（超时释放）三处共用，避免逻辑漂移。
package seckillkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/redis/go-redis/v9"

	"seckill/kitex_gen/product"
	"seckill/pkg/config"
	"seckill/pkg/consts"
	"seckill/pkg/redisx"
)

// ErrNotWarmed 活动尚未同步到 Redis。
var ErrNotWarmed = errors.New("seckill activity not warmed up")

// Snapshot 活动在 Redis 中的快照结构。
type Snapshot struct {
	ActivityID    int64  `json:"activityId"`
	ProductID     int64  `json:"productId"`
	ProductName   string `json:"productName"`
	Cover         string `json:"cover"`
	OriginalPrice int64  `json:"originalPrice"`
	SeckillPrice  int64  `json:"seckillPrice"`
	TotalStock    int32  `json:"totalStock"`
	StartTimeMs   int64  `json:"startTimeMs"`
	EndTimeMs     int64  `json:"endTimeMs"`
	Status        int32  `json:"status"`
}

// DurationSeconds 活动时长（秒），至少 1 秒，避免除零。
func (s *Snapshot) DurationSeconds() float64 {
	d := float64(s.EndTimeMs-s.StartTimeMs) / 1000.0
	if d < 1 {
		return 1
	}
	return d
}

// InWindow 当前是否在活动时间窗内。
func (s *Snapshot) InWindow(now time.Time) bool {
	ms := now.UnixMilli()
	return ms >= s.StartTimeMs && ms < s.EndTimeMs
}

// Syncer 负责把 MySQL 中的活动信息同步到 Redis。
type Syncer struct {
	Redis *redisx.Client
	Cfg   *config.Config
}

// NewSyncer 构造同步器。
func NewSyncer(rdb *redisx.Client, cfg *config.Config) *Syncer {
	return &Syncer{Redis: rdb, Cfg: cfg}
}

// Sync 同步单个活动：
//  1. SET 活动快照 JSON（TTL = end_time - now + buffer）
//  2. SETNX 库存 —— 只初始化，绝不覆盖，否则会把用户已扣减的库存刷回去造成超卖
//  3. SADD 活动索引
func (s *Syncer) Sync(ctx context.Context, item *product.SeckillSyncItem) error {
	if item == nil || item.ActivityId <= 0 {
		return fmt.Errorf("invalid sync item")
	}

	ttl := time.Until(time.UnixMilli(item.EndTimeMs)) + time.Duration(s.Cfg.Seckill.SnapshotTTLBufferSeconds)*time.Second
	if ttl <= 0 {
		// 活动已结束：清理 Redis 残留
		return s.Remove(ctx, item.ActivityId)
	}

	snap := Snapshot{
		ActivityID:    item.ActivityId,
		ProductID:     item.ProductId,
		ProductName:   item.ProductName,
		Cover:         item.Cover,
		OriginalPrice: item.OriginalPrice,
		SeckillPrice:  item.SeckillPrice,
		TotalStock:    item.TotalStock,
		StartTimeMs:   item.StartTimeMs,
		EndTimeMs:     item.EndTimeMs,
		Status:        item.Status,
	}
	raw, err := json.Marshal(&snap)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	pipe := s.Redis.Raw().Pipeline()
	pipe.Set(ctx, consts.SeckillActKey(item.ActivityId), raw, ttl)
	// SETNX：已存在（可能正在被扣减）则保持原值
	pipe.SetNX(ctx, consts.SeckillStockKey(item.ActivityId), item.Stock, ttl)
	pipe.Expire(ctx, consts.SeckillStockKey(item.ActivityId), ttl)
	pipe.Expire(ctx, consts.SeckillUsersKey(item.ActivityId), ttl)
	pipe.SAdd(ctx, consts.SeckillActiveSet, item.ActivityId)
	if _, err = pipe.Exec(ctx); err != nil {
		return fmt.Errorf("sync activity %d: %w", item.ActivityId, err)
	}
	return nil
}

// ForceSyncStock 强制用指定剩余库存覆盖 Redis（仅对账修复时使用）。
func (s *Syncer) ForceSyncStock(ctx context.Context, activityID int64, remain int32) error {
	key := consts.SeckillStockKey(activityID)
	ttl, err := s.Redis.Raw().TTL(ctx, key).Result()
	if err != nil {
		return err
	}
	if ttl <= 0 {
		return fmt.Errorf("stock key %s has no ttl, skip force sync", key)
	}
	return s.Redis.Raw().Set(ctx, key, remain, ttl).Err()
}

// Remove 活动结束时清理 Redis 中的活动数据。
func (s *Syncer) Remove(ctx context.Context, activityID int64) error {
	pipe := s.Redis.Raw().Pipeline()
	pipe.Del(ctx, consts.SeckillActKey(activityID))
	pipe.SRem(ctx, consts.SeckillActiveSet, activityID)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("remove activity %d: %w", activityID, err)
	}
	return nil
}

// Load 读取活动快照。
func (s *Syncer) Load(ctx context.Context, activityID int64) (*Snapshot, error) {
	raw, err := s.Redis.Raw().Get(ctx, consts.SeckillActKey(activityID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrNotWarmed
		}
		return nil, err
	}
	var snap Snapshot
	if err = json.Unmarshal([]byte(raw), &snap); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot of activity %d: %w", activityID, err)
	}
	return &snap, nil
}

// LoadOrNil 读取活动快照与剩余库存，未预热或读取失败时返回 (nil, 0) 且不报错。
func (s *Syncer) LoadOrNil(ctx context.Context, activityID int64) (*Snapshot, int32) {
	snap, err := s.Load(ctx, activityID)
	if err != nil {
		if !errors.Is(err, ErrNotWarmed) {
			LogWarn("load snapshot of activity %d failed: %v", activityID, err)
		}
		return nil, 0
	}
	remain, err := s.RemainStock(ctx, activityID)
	if err != nil {
		return snap, 0
	}
	return snap, remain
}

// RemainStock 读取剩余库存，key 不存在返回 ErrNotWarmed。
func (s *Syncer) RemainStock(ctx context.Context, activityID int64) (int32, error) {
	val, err := s.Redis.Raw().Get(ctx, consts.SeckillStockKey(activityID)).Int()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, ErrNotWarmed
		}
		return 0, err
	}
	return int32(val), nil
}

// RateLimitParams 根据活动快照计算商品维度令牌桶参数。
//
//	capacity = 库存 × capacityFactor   —— 桶够大，让真正的抢购流量能进来
//	rate     = 库存 / 活动时长 × rateFactor —— 令牌按活动时长均匀发放，避免瞬间打满
func RateLimitParams(cfg *config.Config, snap *Snapshot, remain int32) (capacity, rate float64) {
	ac := cfg.RateLimit.Activity
	if snap == nil {
		capacity = ac.FallbackCapacity
		rate = ac.FallbackRate
	} else {
		base := float64(snap.TotalStock)
		capacity = base * ac.CapacityFactor
		rate = base / snap.DurationSeconds() * ac.RateFactor
	}
	if capacity < float64(remain) {
		capacity = float64(remain)
	}
	if capacity < 1 {
		capacity = 1
	}
	if rate < 1 {
		rate = 1
	}
	return capacity, rate
}

// LogWarn 统一日志前缀，便于检索。
func LogWarn(format string, args ...interface{}) {
	klog.Warnf("[seckillkit] "+format, args...)
}
