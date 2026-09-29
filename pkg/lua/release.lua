-- 库存释放（超时未支付 / 主动取消 / 订单创建失败回滚）
-- 两个不变式：
--   1) 已支付的订单绝不能被超时任务回滚 —— 靠 ZREM 延时队列是否成功来判定
--   2) 同一次秒杀资格绝不能被回补两次     —— 靠 SREM 幂等集合是否成功来判定
--
-- KEYS[1] sk:seckill:stock:{actId}
-- KEYS[2] sk:seckill:user:{actId}
-- KEYS[3] sk:order:delay
-- ARGV[1] userId
-- ARGV[2] orderNo（传空字符串表示订单尚未创建，跳过延时队列判定）
-- 返回 {1}=已回补  {0}=无需回补（已支付/已处理/未持有资格）  {-1}=活动已结束，仅清理标记
-- 注意：必须返回 table 而不是裸数字。redisx.run 统一按多值回复解析（toInt64Slice），
-- 裸数字会让回复变成整数而解析出空切片，调用方只能拿到 "unexpected release reply: []"。

local stockKey = KEYS[1]
local usersKey = KEYS[2]
local delayKey = KEYS[3]
local uid      = ARGV[1]
local orderNo  = ARGV[2]

-- 传了订单号时，必须确认延时任务还归我们所有：
-- ZREM 失败说明订单已被支付（order 服务支付成功时会 ZREM），此时绝不能回补库存
if orderNo ~= '' then
  if redis.call('ZREM', delayKey, orderNo) == 0 then
    return {0}
  end
end

-- 未持有秒杀资格标记 → 不重复回补
if redis.call('SREM', usersKey, uid) == 0 then
  return {0}
end

-- 活动已结束且库存 key 已过期，回补会产生永不失效的垃圾 key
if redis.call('EXISTS', stockKey) == 0 then
  return {-1}
end

redis.call('INCR', stockKey)
return {1}
