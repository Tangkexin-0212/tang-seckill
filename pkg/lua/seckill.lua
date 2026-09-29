-- 秒杀核心：活动存在性 + 幂等判重 + 库存扣减
-- 这三步必须在同一个 EVAL 内完成，否则并发下必然出现"同一个人抢到两件"
-- 或"库存扣成负数"。
--
-- KEYS[1] sk:seckill:stock:{actId}   剩余库存
-- KEYS[2] sk:seckill:user:{actId}    已抢到用户集合
-- ARGV[1] userId
-- 返回 {code, remainStock}
--   code: 1=成功  -1=已抢过  -2=库存不足  -3=活动未预热/已结束

local stockKey = KEYS[1]
local usersKey = KEYS[2]
local uid      = ARGV[1]

-- 库存 key 不存在说明活动未预热，或已过 end_time 被 TTL 清理
if redis.call('EXISTS', stockKey) == 0 then
  return {-3, 0}
end

-- 幂等：同一用户只能成功一次
if redis.call('SISMEMBER', usersKey, uid) == 1 then
  return {-1, 0}
end

local stock = tonumber(redis.call('GET', stockKey))
if stock == nil then
  return {-3, 0}
end
if stock <= 0 then
  return {-2, 0}
end

-- DECR 返回扣减后的值，顺便作为剩余库存返回给调用方，省一次 GET
local remain = redis.call('DECR', stockKey)
redis.call('SADD', usersKey, uid)
return {1, remain}
