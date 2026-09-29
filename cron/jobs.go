// 定时任务的具体作业：MySQL → Redis 活动同步、Redis ↔ MySQL 库存对账。
package main

import (
	"context"
	"strconv"

	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/kitex_gen/product/productservice"
	"seckill/kitex_gen/stock/stockservice"
	"seckill/pkg/config"
	"seckill/pkg/consts"
	"seckill/pkg/redisx"
	"seckill/pkg/seckillkit"
	"seckill/pkg/utils"
)

// Jobs 定时作业集合。
type Jobs struct {
	cfg     *config.Config
	rdb     *redisx.Client
	sync    *seckillkit.Syncer
	product productservice.Client
	stock   stockservice.Client
}

// NewJobs 构造作业集合。
func NewJobs(cfg *config.Config, rdb *redisx.Client, sync *seckillkit.Syncer,
	product productservice.Client, stock stockservice.Client) *Jobs {
	return &Jobs{cfg: cfg, rdb: rdb, sync: sync, product: product, stock: stock}
}

// SyncOnce 执行一次全量同步：把 MySQL 中未结束的活动写进 Redis。
//
// 之所以用 SETNX 初始化库存而不是 SET，是因为已预热的活动可能正在被扣减，
// 覆盖写会把已经卖出去的名额刷回来，直接造成超卖。
func (j *Jobs) SyncOnce(ctx context.Context) (int, error) {
	items, err := j.product.ListSyncTargets(ctx, utils.NowMs())
	if err != nil {
		return 0, err
	}

	okCount := 0
	for _, item := range items {
		if item == nil {
			continue
		}
		if err = j.sync.Sync(ctx, item); err != nil {
			klog.Errorf("sync activity %d failed: %v", item.ActivityId, err)
			continue
		}
		okCount++
	}
	klog.Infof("cron sync done: total=%d synced=%d", len(items), okCount)
	return okCount, nil
}

// ReconcileResult 单个活动的一致性快照。
type ReconcileResult struct {
	ActivityID     int64
	TotalStock     int32
	RedisRemain    int32
	DBDeducted     int32
	ExpectedRemain int32
	Consistent     bool
}

// ReconcileOnce 对账：Redis 剩余库存 + MySQL 已扣减数量 应当等于活动总库存。
//
// 注意：由于落库是异步的，短时间内 Redis 会比 MySQL "扣得更多"，
// 这属于正常的在途状态，因此默认只告警不修复；需要修复时显式传 fix=true。
func (j *Jobs) ReconcileOnce(ctx context.Context, fix bool) ([]ReconcileResult, error) {
	ids, err := j.rdb.Raw().SMembers(ctx, consts.SeckillActiveSet).Result()
	if err != nil {
		return nil, err
	}

	results := make([]ReconcileResult, 0, len(ids))
	for _, idStr := range ids {
		activityID, perr := strconv.ParseInt(idStr, 10, 64)
		if perr != nil || activityID <= 0 {
			continue
		}

		snap, lerr := j.sync.Load(ctx, activityID)
		if lerr != nil || snap == nil {
			continue
		}
		redisRemain, rerr := j.sync.RemainStock(ctx, activityID)
		if rerr != nil {
			continue
		}
		dbDeducted, derr := j.stock.DeductedCount(ctx, activityID)
		if derr != nil {
			klog.Warnf("reconcile activity %d: query db deducted failed: %v", activityID, derr)
			continue
		}

		expected := snap.TotalStock - dbDeducted
		r := ReconcileResult{
			ActivityID:     activityID,
			TotalStock:     snap.TotalStock,
			RedisRemain:    redisRemain,
			DBDeducted:     dbDeducted,
			ExpectedRemain: expected,
			Consistent:     redisRemain == expected,
		}
		results = append(results, r)

		if r.Consistent {
			continue
		}

		gap := expected - redisRemain
		klog.Warnf("reconcile mismatch act=%d total=%d redisRemain=%d dbDeducted=%d expectedRemain=%d gap=%d(inflight)",
			activityID, snap.TotalStock, redisRemain, dbDeducted, expected, gap)

		// gap > 0 表示 Redis 扣得比 DB 记录的多，通常是事件还在 Stream 里排队，属正常在途；
		// gap < 0 表示 DB 扣得更多（回补未落库或重复扣减），才是真正需要人工介入的异常。
		if fix && gap < 0 {
			if ferr := j.sync.ForceSyncStock(ctx, activityID, expected); ferr != nil {
				klog.Errorf("reconcile fix activity %d failed: %v", activityID, ferr)
			} else {
				klog.Warnf("reconcile fixed activity %d stock from %d to %d", activityID, redisRemain, expected)
			}
		}
	}

	klog.Infof("cron reconcile done: activities=%d", len(results))
	return results, nil
}

// CleanupExpired 清理活动结束后残留的 Redis key（幂等集合、活动快照）。
func (j *Jobs) CleanupExpired(ctx context.Context, nowMs int64) (int, error) {
	ids, err := j.rdb.Raw().SMembers(ctx, consts.SeckillActiveSet).Result()
	if err != nil {
		return 0, err
	}

	cleaned := 0
	for _, idStr := range ids {
		activityID, perr := strconv.ParseInt(idStr, 10, 64)
		if perr != nil || activityID <= 0 {
			continue
		}
		snap, lerr := j.sync.Load(ctx, activityID)
		if lerr != nil || snap == nil {
			// 快照已过期但活动索引还在 → 清理索引
			if rerr := j.sync.Remove(ctx, activityID); rerr == nil {
				cleaned++
			}
			continue
		}
		if snap.EndTimeMs > 0 && nowMs >= snap.EndTimeMs {
			if rerr := j.sync.Remove(ctx, activityID); rerr == nil {
				cleaned++
			}
		}
	}
	if cleaned > 0 {
		klog.Infof("cron cleanup done: removed=%d", cleaned)
	}
	return cleaned, nil
}
