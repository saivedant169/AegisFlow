package provider

import (
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/saivedant169/AegisFlow/internal/config"
)

type HTTPStatusError struct {
	StatusCode int
	Body       string
	Header     http.Header
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("provider returned status %d: %s", e.StatusCode, e.Body)
}

type retryPolicy struct {
	maxAttempts int
	initial     time.Duration
	max         time.Duration
	multiplier  float64
	jitter      bool
	retryable   map[int]struct{}
	rng         *rand.Rand
}

func newRetryPolicy(cfg config.RetryConfig) retryPolicy {
	policy := retryPolicy{
		maxAttempts: cfg.MaxAttempts,
		initial:     cfg.InitialBackoff,
		max:         cfg.MaxBackoff,
		multiplier:  cfg.BackoffMultiplier,
		jitter:      cfg.Jitter,
		retryable:   make(map[int]struct{}, len(cfg.RetryableStatusCodes)),
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	if policy.maxAttempts <= 0 {
		policy.maxAttempts = 1
	}
	if policy.initial <= 0 {
		policy.initial = 200 * time.Millisecond
	}
	if policy.max <= 0 {
		policy.max = 5 * time.Second
	}
	if policy.multiplier <= 0 {
		policy.multiplier = 2.0
	}
	for _, code := range cfg.RetryableStatusCodes {
		policy.retryable[code] = struct{}{}
	}
	if len(policy.retryable) == 0 {
		for _, code := range []int{429, 500, 502, 503, 504} {
			policy.retryable[code] = struct{}{}
		}
	}
	return policy
}

func (p retryPolicy) shouldRetry(status int) bool {
	if p.maxAttempts <= 1 {
		return false
	}
	_, ok := p.retryable[status]
	return ok
}

func (p retryPolicy) delayForAttempt(attempt int, header http.Header) time.Duration {
	if retryAfter := parseRetryAfter(header.Get("Retry-After")); retryAfter > 0 {
		return retryAfter
	}

	backoff := float64(p.initial)
	if attempt > 1 {
		backoff *= math.Pow(p.multiplier, float64(attempt-1))
	}
	delay := time.Duration(backoff)
	if delay > p.max {
		delay = p.max
	}
	if p.jitter && delay > 0 {
		delay = time.Duration(p.rng.Int63n(int64(delay) + 1))
	}
	return delay
}

func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		if delay := time.Until(when); delay > 0 {
			return delay
		}
	}
	return 0
}
