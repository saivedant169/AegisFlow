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
	mu       sync.RWMutex
	windows  map[string]*window
	limit    int
	period   time.Duration
	stop     chan struct{}
	stopOnce sync.Once
}

func NewMemoryLimiter(limit int, period time.Duration) *MemoryLimiter {
	m := &MemoryLimiter{
		windows: make(map[string]*window),
		limit:   limit,
		period:  period,
		stop:    make(chan struct{}),
	}
	// Without a sweep the windows map grows once per distinct key (tenant, IP,
	// ...) and never shrinks. Evict windows whose period has elapsed; the next
	// request for that key just recreates one.
	go m.janitor()
	return m
}

func (m *MemoryLimiter) janitor() {
	t := time.NewTicker(m.period)
	defer t.Stop()
	for {
		select {
		case <-m.stop:
			return
		case now := <-t.C:
			m.cleanup(now)
		}
	}
}

func (m *MemoryLimiter) cleanup(now time.Time) {
	m.mu.Lock()
	for k, w := range m.windows {
		w.mu.Lock()
		expired := now.After(w.resetAt)
		w.mu.Unlock()
		if expired {
			delete(m.windows, k)
		}
	}
	m.mu.Unlock()
}

// Stop ends the background eviction loop. Safe to call multiple times and from
// multiple goroutines (the select/default close was racy under concurrent Stop).
func (m *MemoryLimiter) Stop() {
	m.stopOnce.Do(func() { close(m.stop) })
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
