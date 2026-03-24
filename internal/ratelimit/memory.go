package ratelimit

import (
	"sync"
	"time"
)

type window struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
}

type MemoryLimiter struct {
	mu      sync.RWMutex
	windows map[string]*window
	limit   int
	period  time.Duration
}

func NewMemoryLimiter(limit int, period time.Duration) *MemoryLimiter {
	return &MemoryLimiter{
		windows: make(map[string]*window),
		limit:   limit,
		period:  period,
	}
}

func (m *MemoryLimiter) Allow(key string, cost int) (bool, error) {
	w := m.getOrCreate(key)

	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	if now.After(w.resetAt) {
		w.count = 0
		w.resetAt = now.Add(m.period)
	}

	if w.count+cost > m.limit {
		return false, nil
	}

	w.count += cost
	return true, nil
}

func (m *MemoryLimiter) getOrCreate(key string) *window {
	m.mu.RLock()
	w, ok := m.windows[key]
	m.mu.RUnlock()
	if ok {
		return w
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if w, ok := m.windows[key]; ok {
		return w
	}

	w = &window{resetAt: time.Now().Add(m.period)}
	m.windows[key] = w
	return w
}
