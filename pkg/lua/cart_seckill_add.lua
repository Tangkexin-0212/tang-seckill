-- 秒杀加购：幂等，数量恒为 1
-- 这里是"保证购物车数量正确"的关键：
--   必须用 HSETNX 而不是 HINCRBY。
--   HINCRBY 在重复请求下会把 qty 累加成 2、3、4……，而 HSETNX 只在
--   field 不存在时写入，天然做到"重复调用结果一致"。
--
-- KEYS[1] sk:cart:{uid}
-- ARGV[1] productId
-- ARGV[2] qty（固定为 1）
-- 返回 {created, qty}  created: 1=本次新建  0=已存在（幂等命中）

local cartKey = KEYS[1]
local pid     = ARGV[1]
local qty     = tonumber(ARGV[2])

local created = redis.call('HSETNX', cartKey, pid, tostring(qty))
if created == 1 then
  return {1, qty}
end

local cur = tonumber(redis.call('HGET', cartKey, pid) or tostring(qty))
return {0, cur}
