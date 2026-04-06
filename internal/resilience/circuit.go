package resilience

import (
	"sync"
	"time"
)

const (
	CircuitClosed   = "closed"
	CircuitOpen     = "open"
	CircuitHalfOpen = "half-open"
)

// CircuitBreaker implements the circuit breaker pattern for downstream
// connectors. When failures exceed a threshold the circuit opens and
// rejects calls until a reset period elapses.
type CircuitBreaker struct {
	mu         sync.Mutex
	name       string
	state      string
	failures   int
	threshold  int
	resetAfter time.Duration
	lastFail   time.Time
}

// NewCircuitBreaker creates a circuit breaker that opens after `threshold`
// consecutive failures and stays open for `resetAfter` before transitioning
// to half-open.
func NewCircuitBreaker(name string, threshold int, resetAfter time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		name:       name,
		state:      CircuitClosed,
		threshold:  threshold,
		resetAfter: resetAfter,
	}
}

// Allow returns true if the circuit allows a request through.
// - Closed: always allows.
// - Open: allows only after resetAfter has elapsed (transitions to half-open).
// - Half-open: allows one probe request.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFail) >= cb.resetAfter {
			cb.state = CircuitHalfOpen
			return true
		}
		return false
	case CircuitHalfOpen:
		// Allow one probe request.
		return true
	}
	return false
}

// RecordSuccess records a successful call, resetting the circuit to closed.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = CircuitClosed
}

// RecordFailure records a failed call. If failures reach the threshold the
// circuit transitions to open.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFail = time.Now()
	if cb.failures >= cb.threshold {
		cb.state = CircuitOpen
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// Name returns the circuit breaker's name.
func (cb *CircuitBreaker) Name() string {
	return cb.name
}
