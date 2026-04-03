package ratelimit

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	//go:embed script/fixed_window.lua
	fixedWindowRateLimitLua string
	//go:embed script/sliding_window.lua
	slidingWindowRateLimitLua string
)

type LimitMode string

const (
	ModeFixed   LimitMode = "fixed"
	ModeSliding LimitMode = "sliding"
)

type LimiterConfig struct {
	limit  int
	period time.Duration
	mode   LimitMode
}

type LimiterOption interface {
	Apply(config LimiterConfig) LimiterConfig
}

type LimiterOptionFunc func(config LimiterConfig) LimiterConfig

func (o LimiterOptionFunc) Apply(config LimiterConfig) LimiterConfig {
	return o(config)
}

func WithLimit(limit int) LimiterOption {
	return LimiterOptionFunc(func(config LimiterConfig) LimiterConfig {
		config.limit = limit
		return config
	})
}

func WithPeriod(period time.Duration) LimiterOption {
	return LimiterOptionFunc(func(config LimiterConfig) LimiterConfig {
		config.period = period
		return config
	})
}

func WithMode(mode LimitMode) LimiterOption {
	return LimiterOptionFunc(func(config LimiterConfig) LimiterConfig {
		config.mode = mode
		return config
	})
}

type RedisLimiter struct {
	client *redis.Client
	config LimiterConfig
}

func NewRedisLimiter(addr, password string, db int, opts ...LimiterOption) (*RedisLimiter, error) {
	config := LimiterConfig{}
	for _, opt := range opts {
		config = opt.Apply(config)
	}

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
		config: config,
	}, nil
}

func (r *RedisLimiter) Allow(key string, cost int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	limit := r.config.limit
	period := r.config.period
	key = "ratelimit:" + key

	switch r.config.mode {
	case ModeSliding:
		if cost > limit {
			return false, nil
		}
		now := time.Now()
		return r.eval(ctx, slidingWindowRateLimitLua, key,
			period.Milliseconds(), limit, now.UnixMilli(), cost,
		)
	default:
		return r.eval(ctx, fixedWindowRateLimitLua, key, limit, cost, int(period.Seconds()))
	}
}

func (r *RedisLimiter) eval(ctx context.Context, script, key string, args ...any) (bool, error) {
	result, err := r.client.Eval(ctx, script, []string{key}, args...).Int()
	if err != nil {
		return false, fmt.Errorf("redis eval: %w", err)
	}
	return result == 1, nil
}
