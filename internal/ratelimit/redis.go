package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisLimiter struct {
	client *redis.Client
	limit  int
	period time.Duration
}

func NewRedisLimiter(addr, password string, db, limit int, period time.Duration) (*RedisLimiter, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisLimiter{
		client: client,
		limit:  limit,
		period: period,
	}, nil
}

const luaScript = `
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
`

func (r *RedisLimiter) Allow(key string, cost int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := r.client.Eval(ctx, luaScript, []string{"ratelimit:" + key}, r.limit, cost, int(r.period.Seconds())).Int()
	if err != nil {
		return false, fmt.Errorf("redis eval: %w", err)
	}

	return result == 1, nil
}
