package loadshed

import (
	"context"
	"sync/atomic"
	"time"
)

// Priority represents the priority level of a request.
type Priority int

const (
	PriorityLow    Priority = 0
	PriorityNormal Priority = 1
	PriorityHigh   Priority = 2
)

// ParsePriority converts a string to a Priority level.
func ParsePriority(s string) Priority {
	switch s {
	case "high":
		return PriorityHigh
	case "low":
		return PriorityLow
	default:
		return PriorityNormal
	}
}

// Config holds the configuration for the load shedder.
type Config struct {
	MaxConcurrent int64         // hard limit on simultaneous in-flight requests
	QueueSize     int           // how many requests can wait in queue
	QueueTimeout  time.Duration // max time a request waits in queue before 503
}

// Shedder manages admission control with priority-based load shedding.
type Shedder struct {
	cfg      Config
	inflight atomic.Int64
	queue    chan struct{} // buffered channel used as a semaphore for queued slots
}

// New creates a new Shedder with the given configuration.
func New(cfg Config) *Shedder {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 100
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 50
	}
	if cfg.QueueTimeout <= 0 {
		cfg.QueueTimeout = 10 * time.Second
	}
	return &Shedder{
		cfg:   cfg,
		queue: make(chan struct{}, cfg.QueueSize),
	}
}

// Result describes the outcome of an Acquire call.
type Result int

const (
	Admitted Result = iota
	Shed
	QueueTimeout
)

// Acquire attempts to admit a request. On success it returns Admitted and a
// release function that MUST be called when the request completes. On failure
// it returns Shed or QueueTimeout with a nil release function.
func (s *Shedder) Acquire(ctx context.Context, priority Priority) (Result, func()) {
	max := s.cfg.MaxConcurrent

	// Low priority: shed at 80% capacity.
	if priority == PriorityLow {
		threshold := max * 80 / 100
		if s.inflight.Load() >= threshold {
			return Shed, nil
		}
	}

	// Try direct admission (always possible if under max).
	if s.tryAdmit(max) {
		return Admitted, s.releaseFunc()
	}

	// High priority requests never queue -- they are only admitted directly.
	// If we're at capacity and high priority can't get in, shed.
	if priority == PriorityHigh {
		// Try once more with an extra slot allowance for high priority.
		// High priority can go up to MaxConcurrent (they bypass the queue,
		// but still respect the hard ceiling).
		return Shed, nil
	}

	// Normal priority: attempt to enter the queue.
	select {
	case s.queue <- struct{}{}:
		// Got a queue slot, now wait for capacity.
	default:
		// Queue is full.
		return Shed, nil
	}

	// Wait for an inflight slot to open up or timeout.
	deadline := s.cfg.QueueTimeout
	timer := time.NewTimer(deadline)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			// Drain our queue slot.
			<-s.queue
			return QueueTimeout, nil
		case <-timer.C:
			<-s.queue
			return QueueTimeout, nil
		default:
			if s.tryAdmit(max) {
				<-s.queue // release queue slot
				return Admitted, s.releaseFunc()
			}
			// Brief sleep to avoid busy-spin.
			time.Sleep(time.Millisecond)
		}
	}
}

// Inflight returns the current number of in-flight requests.
func (s *Shedder) Inflight() int64 {
	return s.inflight.Load()
}

// QueueLen returns the current number of requests waiting in the queue.
func (s *Shedder) QueueLen() int {
	return len(s.queue)
}

func (s *Shedder) tryAdmit(max int64) bool {
	for {
		cur := s.inflight.Load()
		if cur >= max {
			return false
		}
		if s.inflight.CompareAndSwap(cur, cur+1) {
			return true
		}
	}
}

func (s *Shedder) releaseFunc() func() {
	var released atomic.Bool
	return func() {
		if released.CompareAndSwap(false, true) {
			s.inflight.Add(-1)
		}
	}
}
