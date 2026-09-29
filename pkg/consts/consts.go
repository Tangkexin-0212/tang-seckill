// Package consts 汇总服务名、库名与 Redis Key 命名规则。
//
// Redis Key 全表（前缀统一为 sk:）：
//
//	sk:seckill:act:{actId}       String  活动快照 JSON，TTL = end_time - now + buffer
//	sk:seckill:stock:{actId}     String  剩余库存（整型），由 cron SETNX 初始化
//	sk:seckill:user:{actId}      Set     已抢到的 userId（幂等集合）
//	sk:seckill:active            Set     进行中的活动 ID
//	sk:rl:act:{actId}            Hash    商品维度令牌桶
//	sk:rl:user:{uid}             Hash    用户维度令牌桶
//	sk:rl:global                 Hash    全局兜底令牌桶
//	sk:cart:{uid}                Hash    field=productId value=qty
//	sk:cart:snap:{uid}           Hash    field=productId value=商品快照 JSON
//	sk:user:token:{token}        String  token -> uid
//	sk:stream:seckill            Stream  秒杀成功事件
//	sk:stream:seckill:dlq        Stream  死信
//	sk:order:delay               ZSet    member=orderNo score=到期毫秒时间戳
//	sk:order:paid                Set     已支付 orderNo
package consts

import "strconv"

// 服务名（同时作为 etcd 注册的服务标识）
const (
	ServiceUser    = "user"
	ServiceProduct = "product"
	ServiceStock   = "stock"
	ServiceCart    = "cart"
	ServiceSeckill = "seckill"
	ServiceOrder   = "order"
	ServiceGateway = "gateway"
)

// 数据库名
const (
	DBUser    = "user_db"
	DBProduct = "product_db"
	DBStock   = "stock_db"
	DBOrder   = "order_db"
)

// KeyPrefix Redis key 统一前缀
const KeyPrefix = "sk:"

// SeckillActiveSet 进行中的活动 ID 集合
const SeckillActiveSet = KeyPrefix + "seckill:active"

// RateLimitGlobalKey 全局兜底令牌桶
const RateLimitGlobalKey = KeyPrefix + "rl:global"

// StreamSeckill 秒杀成功事件流
const StreamSeckill = KeyPrefix + "stream:seckill"

// StreamSeckillDLQ 死信流
const StreamSeckillDLQ = KeyPrefix + "stream:seckill:dlq"

// StreamSeckillGroup 秒杀事件消费组
const StreamSeckillGroup = "g:order"

// OrderDelayZSet 订单延时队列
const OrderDelayZSet = KeyPrefix + "order:delay"

// OrderPaidSet 已支付订单集合，供延时任务快速跳过
const OrderPaidSet = KeyPrefix + "order:paid"

// EventSeckillSuccess Stream 事件类型
const EventSeckillSuccess = "seckill_success"

// EventReleaseStock Stream 事件类型（超时/取消释放）
const EventReleaseStock = "release_stock"

// SeckillActKey 活动快照 key
func SeckillActKey(activityID int64) string {
	return KeyPrefix + "seckill:act:" + strconv.FormatInt(activityID, 10)
}

// SeckillStockKey 库存 key
func SeckillStockKey(activityID int64) string {
	return KeyPrefix + "seckill:stock:" + strconv.FormatInt(activityID, 10)
}

// SeckillUsersKey 幂等用户集合 key
func SeckillUsersKey(activityID int64) string {
	return KeyPrefix + "seckill:user:" + strconv.FormatInt(activityID, 10)
}

// RateLimitActKey 商品维度令牌桶 key
func RateLimitActKey(activityID int64) string {
	return KeyPrefix + "rl:act:" + strconv.FormatInt(activityID, 10)
}

// RateLimitUserKey 用户维度令牌桶 key
func RateLimitUserKey(uid int64) string {
	return KeyPrefix + "rl:user:" + strconv.FormatInt(uid, 10)
}

// CartKey 购物车 Hash key
func CartKey(uid int64) string {
	return KeyPrefix + "cart:" + strconv.FormatInt(uid, 10)
}

// CartSnapKey 购物车商品快照 Hash key
func CartSnapKey(uid int64) string {
	return KeyPrefix + "cart:snap:" + strconv.FormatInt(uid, 10)
}

// UserTokenKey token key
func UserTokenKey(token string) string {
	return KeyPrefix + "user:token:" + token
}

// MQRetryKey 单条 Stream 消息的重试计数 key
func MQRetryKey(msgID string) string {
	return KeyPrefix + "mq:retry:" + msgID
}
