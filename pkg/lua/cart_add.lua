-- 普通加购（可累加，但有单商品上限校验）
-- KEYS[1] sk:cart:{uid}
-- ARGV[1] productId
-- ARGV[2] delta 新增数量
-- ARGV[3] maxPerItem 单商品数量上限
-- 返回 {code, qty}  code: 1=成功  -1=超出上限  -2=增量非法

local cartKey = KEYS[1]
local pid     = ARGV[1]
local add     = tonumber(ARGV[2])
local maxQty  = tonumber(ARGV[3])

if add == nil or add <= 0 then
  return {-2, 0}
end

local cur = tonumber(redis.call('HGET', cartKey, pid) or '0')
local target = cur + add

if target > maxQty then
  return {-1, cur}
end

redis.call('HSET', cartKey, pid, tostring(target))
return {1, target}
