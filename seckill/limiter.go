package main

import (
	"context"

	"github.com/cloudwego/kitex/pkg/klog"

	"seckill/pkg/config"
	"seckill/pkg/consts"
	"seckill/pkg/errno"
	"seckill/pkg/redisx"
	"seckill/pkg/seckillkit"
)

// Limiter 基于 Redis 令牌桶的三层限流：
//
//	第一层 用户维度  sk:rl:user:{uid}   桶最小，最先挡掉脚本刷单
//	第二层 商品维度  sk:rl:act:{actId}  容量与速率随活动库存和时长自适应
//	第三层 全局兜底  sk:rl:global       防止单机/单活动异常把整个集群打挂
type Limiter struct {
	rdb *redisx.Client
	cfg *config.Config
}

// NewLimiter 构造限流器。
func NewLimiter(rdb *redisx.Client, cfg *config.Config) *Limiter {
	return &Limiter{rdb: rdb, cfg: cfg}
}

// Allow 依次通过三层令牌桶，任一层拒绝即返回限流错误。
func (l *Limiter) Allow(ctx context.Context, uid, activityID int64, snap *seckillkit.Snapshot, remain int32) *errno.Err {
	if e := l.allowUser(ctx, uid); e != nil {
		return e
	}
	if e := l.allowActivity(ctx, activityID, snap, remain); e != nil {
		return e
	}
	return l.allowGlobal(ctx)
}

func (l *Limiter) allowUser(ctx context.Context, uid int64) *errno.Err {
	uc := l.cfg.RateLimit.User
	if !uc.Enabled || uid <= 0 {
		return nil
	}
	return l.one(ctx, consts.RateLimitUserKey(uid), uc.Capacity, uc.RatePerSecond, "user")
}

func (l *Limiter) allowActivity(ctx context.Context, activityID int64, snap *seckillkit.Snapshot, remain int32) *errno.Err {
	if !l.cfg.RateLimit.Activity.Enabled || activityID <= 0 {
		return nil
	}
	// 允许调用方不显式传快照：从 context 里取中间件预读的结果，避免重复访问 Redis
	if snap == nil {
		if s, r, ok := seckillkit.SnapshotFrom(ctx); ok {
			snap, remain = s, r
		}
	}
	capacity, rate := seckillkit.RateLimitParams(l.cfg, snap, remain)
	return l.one(ctx, consts.RateLimitActKey(activityID), capacity, rate, "activity")
}

func (l *Limiter) allowGlobal(ctx context.Context) *errno.Err {
	gc := l.cfg.RateLimit.Global
	if gc.Capacity <= 0 {
		return nil
	}
	return l.one(ctx, consts.RateLimitGlobalKey, gc.Capacity, gc.RatePerSecond, "global")
}

func (l *Limiter) one(ctx context.Context, key string, capacity, rate float64, scope string) *errno.Err {
	allowed, remain, err := l.rdb.Allow(ctx, key, capacity, rate, 1)
	if err != nil {
		// 限流组件本身故障时不能成为新的故障点：Fail-Open 放行并告警
		klog.Warnf("ratelimit[%s] redis error, key=%s, failOpen=%v, err=%v",
			scope, key, l.cfg.RateLimit.FailOpen, err)
		if l.cfg.RateLimit.FailOpen {
			return nil
		}
		return errno.WithMsg(errno.Internal, "限流服务异常")
	}
	if !allowed {
		klog.Infof("ratelimit[%s] rejected, key=%s, remain=%.0f", scope, key, remain)
		return errno.RateLimited
	}
	return nil
}
