-- 令牌桶限流
-- 单次 EVAL 内完成"读桶 -> 按时间差补令牌 -> 判断 -> 写回"，天然原子，
-- 不存在 INCR+EXPIRE 固定窗口在临界点的双倍流量问题。
--
-- KEYS[1] 桶 key（Hash: tokens / ts）
-- ARGV[1] capacity  桶容量
-- ARGV[2] rate      令牌补充速率（个/秒）
-- ARGV[3] now_ms    当前毫秒时间戳
-- ARGV[4] requested 本次申请令牌数
-- 返回 {allowed, remain}  allowed: 1 放行 / 0 拒绝

local key       = KEYS[1]
local capacity  = tonumber(ARGV[1])
local rate      = tonumber(ARGV[2])
local now       = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])

if capacity == nil or capacity <= 0 or rate == nil or rate <= 0 then
  return {1, 0}
end
if requested == nil or requested <= 0 then
  requested = 1
end

local data   = redis.call('HMGET', key, 'tokens', 'ts')
local tokens = tonumber(data[1])
local ts     = tonumber(data[2])

-- 冷启动：桶满
if tokens == nil or ts == nil then
  tokens = capacity
  ts = now
end

-- 时钟回拨保护
if now < ts then
  ts = now
end

local delta = now - ts
if delta > 0 then
  tokens = math.min(capacity, tokens + delta * rate / 1000.0)
  ts = now
end

local allowed = 0
if tokens >= requested then
  tokens = tokens - requested
  allowed = 1
end

redis.call('HSET', key, 'tokens', tostring(tokens), 'ts', tostring(ts))

-- TTL = 从空桶补满所需时间的 2 倍，自动清理冷桶
local ttl = math.ceil(capacity / rate * 1000) * 2
if ttl < 1000 then ttl = 1000 end
redis.call('PEXPIRE', key, ttl)

return {allowed, math.floor(tokens)}
