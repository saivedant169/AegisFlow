package ratelimit

type Limiter interface {
	Allow(key string, cost int) (bool, error)
}
