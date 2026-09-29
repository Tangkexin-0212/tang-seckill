// 延时队列扫描：把超时未支付的订单释放掉，库存重新回到池子里。
package main

import (
	"context"
	"strconv"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/redis/go-redis/v9"

	"seckill/pkg/consts"
	"seckill/pkg/redisx"
	"seckill/pkg/seckillkit"
	"seckill/pkg/utils"
)

// delayBatch 单轮最多处理的到期订单数，避免一次扫描阻塞太久。
const delayBatch = 200

// DelayScanner 扫描 ZSet 延时队列 sk:order:delay。
//
// 多实例并发扫描是安全的：release Lua 里用 ZREM 的返回值做判定，
// 只有真正摘除成功的那个实例才会回补库存，天然互斥。
type DelayScanner struct {
	rdb *redisx.Client
	rel *seckillkit.Releaser
}

// NewDelayScanner 构造扫描器。
func NewDelayScanner(rdb *redisx.Client, rel *seckillkit.Releaser) *DelayScanner {
	return &DelayScanner{rdb: rdb, rel: rel}
}

// Run 按 interval 周期扫描直到 ctx 取消。
func (d *DelayScanner) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	klog.Infof("delay scanner started: interval=%s", interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.scanOnce(ctx)
		}
	}
}

func (d *DelayScanner) scanOnce(ctx context.Context) {
	nowMs := utils.NowMs()
	orderNos, err := d.rdb.Raw().ZRangeByScore(ctx, consts.OrderDelayZSet, &redis.ZRangeBy{
		Min:    "0",
		Max:    strconv.FormatInt(nowMs, 10),
		Offset: 0,
		Count:  delayBatch,
	}).Result()
	if err != nil {
		if ctx.Err() == nil {
			klog.Errorf("scan delay zset failed: %v", err)
		}
		return
	}
	if len(orderNos) == 0 {
		return
	}

	released := 0
	for _, orderNo := range orderNos {
		if ctx.Err() != nil {
			return
		}
		out, rerr := d.rel.ReleaseByOrderNo(ctx, orderNo, "pay timeout")
		if rerr != nil {
			// 订单服务暂时不可用时保留在 ZSet 里，下一轮自动重试
			klog.Warnf("release expired order %s failed: %v", orderNo, rerr)
			continue
		}
		if out != seckillkit.ReleaseNone {
			released++
		}
	}
	if released > 0 {
		klog.Infof("delay scanner released %d expired order(s), scanned=%d", released, len(orderNos))
	}
}
