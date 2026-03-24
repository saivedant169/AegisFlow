package router

import (
	"sync"
	"time"
)

type CircuitBreaker struct {
	mu          sync.RWMutex
	failures    map[string]int
	lastFailure map[string]time.Time
	threshold   int
	resetAfter  time.Duration
}

func NewCircuitBreaker(threshold int, resetAfter time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failures:    make(map[string]int),
		lastFailure: make(map[string]time.Time),
		threshold:   threshold,
		resetAfter:  resetAfter,
	}
}

func (cb *CircuitBreaker) IsOpen(providerName string) bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	count, ok := cb.failures[providerName]
	if !ok || count < cb.threshold {
		return false
	}

	lastFail, ok := cb.lastFailure[providerName]
	if !ok {
		return false
	}

	if time.Since(lastFail) > cb.resetAfter {
		return false
	}

	return true
}

func (cb *CircuitBreaker) RecordFailure(providerName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures[providerName]++
	cb.lastFailure[providerName] = time.Now()
}

func (cb *CircuitBreaker) RecordSuccess(providerName string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures[providerName] = 0
	delete(cb.lastFailure, providerName)
}
