local key = KEYS[1]
local limit = tonumber(ARGV[1])
local cost = tonumber(ARGV[2])
local period = tonumber(ARGV[3])

local current = tonumber(redis.call("GET", key) or "0")
if current + cost > limit then
    return 0
end

if current == 0 then
    redis.call("SET", key, cost, "EX", period)
else
    redis.call("INCRBY", key, cost)
end

return 1
