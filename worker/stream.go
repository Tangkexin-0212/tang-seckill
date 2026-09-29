// 秒杀事件消费者：把 Redis 中已扣减的库存异步落进 MySQL（最终一致性）。
package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"seckill/kitex_gen/stock/stockservice"
	"seckill/pkg/config"
	"seckill/pkg/consts"
	"seckill/pkg/models"
	"seckill/pkg/mysqlx"
	"seckill/pkg/redisx"
	"seckill/pkg/seckillkit"
)

// claimMinIdle 崩溃消费者遗留消息的接管阈值。
const claimMinIdle = 30 * time.Second

// StreamConsumer 消费 sk:stream:seckill。
//
//	at-least-once 语义 + 两道幂等：
//	  1. mq_consume_log 的 uk(msg_id, consumer) —— 挡住同一条消息的重复处理
//	  2. stock_flow 的 uk(order_no, type)      —— 即使绕过第一道也只扣一次
type StreamConsumer struct {
	cfg      *config.Config
	rdb      *redisx.Client
	db       *gorm.DB
	stock    stockservice.Client
	consumer string
}

// NewStreamConsumer 构造消费者。
func NewStreamConsumer(cfg *config.Config, rdb *redisx.Client, db *gorm.DB, stock stockservice.Client) *StreamConsumer {
	return &StreamConsumer{
		cfg:      cfg,
		rdb:      rdb,
		db:       db,
		stock:    stock,
		consumer: cfg.Worker.ConsumerName,
	}
}

// Run 阻塞消费直到 ctx 取消。
func (s *StreamConsumer) Run(ctx context.Context) {
	if err := s.ensureGroup(ctx); err != nil {
		klog.Errorf("create consumer group failed: %v", err)
		return
	}
	klog.Infof("stream consumer started: stream=%s group=%s consumer=%s",
		consts.StreamSeckill, consts.StreamSeckillGroup, s.consumer)

	block := time.Duration(s.cfg.Worker.BlockMs) * time.Millisecond
	for {
		if ctx.Err() != nil {
			return
		}
		res, err := s.rdb.Raw().XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consts.StreamSeckillGroup,
			Consumer: s.consumer,
			Streams:  []string{consts.StreamSeckill, ">"},
			Count:    s.cfg.Worker.BatchSize,
			Block:    block,
		}).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue // block 到期无新消息，正常
			}
			if ctx.Err() != nil {
				return
			}
			klog.Errorf("xreadgroup failed: %v", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}

		for _, st := range res {
			for _, msg := range st.Messages {
				s.handle(ctx, msg)
			}
		}
	}
}

// RunClaimLoop 定期接管其它消费者崩溃后滞留的 pending 消息。
// 单实例部署时它同时兜底"进程被 kill 导致消息一直未 ack"的情况。
func (s *StreamConsumer) RunClaimLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.claimOnce(ctx)
		}
	}
}

func (s *StreamConsumer) claimOnce(ctx context.Context) {
	msgs, _, err := s.rdb.Raw().XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   consts.StreamSeckill,
		Group:    consts.StreamSeckillGroup,
		Consumer: s.consumer,
		MinIdle:  claimMinIdle,
		Start:    "0-0",
		Count:    50,
	}).Result()
	if err != nil {
		if ctx.Err() == nil {
			klog.Warnf("xautoclaim failed: %v", err)
		}
		return
	}
	for _, msg := range msgs {
		klog.Warnf("claimed stuck message msgID=%s", msg.ID)
		s.handle(ctx, msg)
	}
}

// ensureGroup 创建消费组（已存在时忽略 BUSYGROUP）。
func (s *StreamConsumer) ensureGroup(ctx context.Context) error {
	err := s.rdb.Raw().XGroupCreateMkStream(ctx,
		consts.StreamSeckill, consts.StreamSeckillGroup, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

func (s *StreamConsumer) handle(ctx context.Context, msg redis.XMessage) {
	payload, _ := msg.Values["payload"].(string)
	ev, err := seckillkit.DecodeEvent(payload)
	if err != nil {
		klog.Errorf("decode event failed msgID=%s: %v", msg.ID, err)
		_ = seckillkit.PublishDLQ(ctx, s.rdb, msg.ID, "decode error: "+err.Error(), payload)
		s.ack(ctx, msg.ID) // 脏数据重试也不会变好，进死信后 ack
		return
	}
	if ev.Type != consts.EventSeckillSuccess {
		klog.Warnf("skip unsupported event type=%s msgID=%s", ev.Type, msg.ID)
		s.ack(ctx, msg.ID)
		return
	}

	dup, err := s.markConsumed(ctx, msg.ID)
	if err != nil {
		// 记录失败时不 ack，交给 pending 重试，避免消息丢失
		klog.Errorf("mark consumed failed msgID=%s: %v", msg.ID, err)
		return
	}
	if dup {
		s.ack(ctx, msg.ID)
		return
	}

	if _, derr := s.stock.Deduct(ctx, ev.ActivityID, ev.UserID, ev.ProductID, ev.OrderNo); derr != nil {
		klog.Errorf("stock deduct failed orderNo=%s msgID=%s: %v", ev.OrderNo, msg.ID, derr)
		// 撤销幂等标记，让下次重试能真正再执行一次扣减
		if uerr := s.unmarkConsumed(ctx, msg.ID); uerr != nil {
			klog.Warnf("unmark consumed failed msgID=%s: %v", msg.ID, uerr)
		}
		if s.retryExceeded(ctx, msg.ID) {
			klog.Errorf("message %s exceeded max retry(%d), moving to DLQ", msg.ID, s.cfg.Worker.MaxRetry)
			_ = seckillkit.PublishDLQ(ctx, s.rdb, msg.ID, "deduct failed: "+derr.Error(), payload)
			s.ack(ctx, msg.ID)
		}
		return
	}

	s.ack(ctx, msg.ID)
}

// markConsumed 写入消费记录，返回是否为重复消费。
func (s *StreamConsumer) markConsumed(ctx context.Context, msgID string) (bool, error) {
	err := s.db.WithContext(ctx).Create(&models.MQConsumeLog{
		MsgID:    msgID,
		Consumer: s.consumer,
	}).Error
	if err == nil {
		return false, nil
	}
	if mysqlx.IsDupErr(err) {
		return true, nil
	}
	return false, err
}

func (s *StreamConsumer) unmarkConsumed(ctx context.Context, msgID string) error {
	return s.db.WithContext(ctx).
		Where("msg_id = ? AND consumer = ?", msgID, s.consumer).
		Delete(&models.MQConsumeLog{}).Error
}

// retryExceeded 统计单条消息的失败次数。
func (s *StreamConsumer) retryExceeded(ctx context.Context, msgID string) bool {
	key := consts.MQRetryKey(msgID)
	n, err := s.rdb.Raw().Incr(ctx, key).Result()
	if err != nil {
		klog.Warnf("incr retry counter %s failed: %v", key, err)
		return false
	}
	_ = s.rdb.Raw().Expire(ctx, key, 10*time.Minute).Err()
	return int(n) > s.cfg.Worker.MaxRetry
}

func (s *StreamConsumer) ack(ctx context.Context, msgID string) {
	if err := s.rdb.Raw().XAck(ctx, consts.StreamSeckill, consts.StreamSeckillGroup, msgID).Err(); err != nil {
		klog.Warnf("xack failed msgID=%s: %v", msgID, err)
	}
}
