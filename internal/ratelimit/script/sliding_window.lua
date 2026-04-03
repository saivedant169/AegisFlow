-- Sliding window rate limit (ZSET, score = event time in ms).
-- KEYS[1]   Redis key for the ZSET
-- ARGV[1]   window length in milliseconds (trim + PEXPIRE)
-- ARGV[2]   max events allowed in the window
-- ARGV[3]   current time in milliseconds
-- ARGV[4]   cost (slots to consume), >= 1
--
-- Returns 1 if allowed, 0 if denied. Matches fixed_window.lua semantics.

local key = KEYS[1]
local window = tonumber(ARGV[1])
local threshold = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local cost = tonumber(ARGV[4])

if cost == nil or cost < 1 then
    return 0
end

local min = now - window
redis.call('ZREMRANGEBYSCORE', key, '-inf', min)
local cnt = redis.call('ZCOUNT', key, '-inf', '+inf')

if cnt + cost > threshold then
    return 0
end

local seq = redis.call('INCR', key .. ':seq')
for i = 1, cost do
    redis.call('ZADD', key, now, seq .. ':' .. i)
end
redis.call('PEXPIRE', key, window)
return 1
