package seckillkit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"

	"seckill/pkg/consts"
	"seckill/pkg/redisx"
)

// Event 秒杀成功事件。秒杀链路只写 Redis，MySQL 由 worker 异步消费这条事件落库，
// 这就是"Redis 为权威库存、MySQL 最终一致"的实现方式。
type Event struct {
	Type        string `json:"type"`
	OrderNo     string `json:"orderNo"`
	UserID      int64  `json:"userId"`
	ActivityID  int64  `json:"activityId"`
	ProductID   int64  `json:"productId"`
	Amount      int64  `json:"amount"`
	CreatedAtMs int64  `json:"createdAtMs"`
}

// PublishEvent 把事件写入 Redis Stream。
func PublishEvent(ctx context.Context, rdb *redisx.Client, ev *Event) error {
	if ev == nil {
		return fmt.Errorf("nil event")
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return rdb.Raw().XAdd(ctx, &redis.XAddArgs{
		Stream: consts.StreamSeckill,
		MaxLen: 100000,
		Approx: true,
		Values: []interface{}{"payload", string(raw)},
	}).Err()
}

// DecodeEvent 解析 Stream 中的事件负载。
func DecodeEvent(payload string) (*Event, error) {
	var ev Event
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}
	return &ev, nil
}

// PublishDLQ 把无法处理的消息写入死信流。
func PublishDLQ(ctx context.Context, rdb *redisx.Client, msgID, reason, payload string) error {
	return rdb.Raw().XAdd(ctx, &redis.XAddArgs{
		Stream: consts.StreamSeckillDLQ,
		MaxLen: 10000,
		Approx: true,
		Values: map[string]interface{}{
			"srcId":   msgID,
			"reason":  reason,
			"payload": payload,
		},
	}).Err()
}
