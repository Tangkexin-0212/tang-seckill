-- 修改购物车数量（qty<=0 视为删除）
-- KEYS[1] sk:cart:{uid}
-- ARGV[1] productId
-- ARGV[2] qty
-- ARGV[3] maxPerItem
-- 返回 {code, qty}  code: 1=已更新  2=已删除  -1=超出上限  -2=参数非法

local cartKey = KEYS[1]
local pid     = ARGV[1]
local qty     = tonumber(ARGV[2])
local maxQty  = tonumber(ARGV[3])

if qty == nil or qty < 0 then
  return {-2, 0}
end

if qty == 0 then
  redis.call('HDEL', cartKey, pid)
  return {2, 0}
end

if qty > maxQty then
  return {-1, qty}
end

redis.call('HSET', cartKey, pid, tostring(qty))
return {1, qty}
