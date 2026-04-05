package loadshed

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdmissionUnderCapacity(t *testing.T) {
	s := New(Config{MaxConcurrent: 10, QueueSize: 5, QueueTimeout: time.Second})

	result, release := s.Acquire(context.Background(), PriorityNormal)
	if result != Admitted {
		t.Fatalf("expected Admitted, got %d", result)
	}
	if release == nil {
		t.Fatal("expected non-nil release function")
	}
	if s.Inflight() != 1 {
		t.Fatalf("expected 1 inflight, got %d", s.Inflight())
	}
	release()
	if s.Inflight() != 0 {
		t.Fatalf("expected 0 inflight after release, got %d", s.Inflight())
	}
}

func TestQueueingAtCapacity(t *testing.T) {
	s := New(Config{MaxConcurrent: 2, QueueSize: 5, QueueTimeout: 2 * time.Second})

	// Fill to capacity.
	var releases []func()
	for i := 0; i < 2; i++ {
		result, rel := s.Acquire(context.Background(), PriorityNormal)
		if result != Admitted {
			t.Fatalf("fill: expected Admitted, got %d", result)
		}
		releases = append(releases, rel)
	}

	// Next request should queue and then be admitted when we release.
	done := make(chan Result, 1)
	go func() {
		r, rel := s.Acquire(context.Background(), PriorityNormal)
		done <- r
		if rel != nil {
			rel()
		}
	}()

	// Give the goroutine a moment to enter the queue.
	time.Sleep(50 * time.Millisecond)
	if s.QueueLen() != 1 {
		t.Fatalf("expected 1 queued, got %d", s.QueueLen())
	}

	// Release one slot.
	releases[0]()

	select {
	case r := <-done:
		if r != Admitted {
			t.Fatalf("queued request: expected Admitted, got %d", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("queued request did not get admitted in time")
	}

	releases[1]()
}

func TestShedLowPriorityAt80Percent(t *testing.T) {
	s := New(Config{MaxConcurrent: 10, QueueSize: 5, QueueTimeout: time.Second})

	// Fill to 80% capacity (8 of 10).
	var releases []func()
	for i := 0; i < 8; i++ {
		result, rel := s.Acquire(context.Background(), PriorityNormal)
		if result != Admitted {
			t.Fatalf("fill: expected Admitted, got %d at i=%d", result, i)
		}
		releases = append(releases, rel)
	}

	// Low priority should be shed at 80%.
	result, rel := s.Acquire(context.Background(), PriorityLow)
	if result != Shed {
		t.Fatalf("low priority at 80%%: expected Shed, got %d", result)
	}
	if rel != nil {
		t.Fatal("expected nil release on shed")
	}

	// Normal priority should still be admitted (under max).
	result, rel = s.Acquire(context.Background(), PriorityNormal)
	if result != Admitted {
		t.Fatalf("normal priority at 80%%: expected Admitted, got %d", result)
	}
	rel()

	for _, r := range releases {
		r()
	}
}

func TestHighPriorityBypass(t *testing.T) {
	s := New(Config{MaxConcurrent: 5, QueueSize: 2, QueueTimeout: time.Second})

	// Fill to capacity.
	var releases []func()
	for i := 0; i < 5; i++ {
		result, rel := s.Acquire(context.Background(), PriorityNormal)
		if result != Admitted {
			t.Fatalf("fill: expected Admitted at i=%d", i)
		}
		releases = append(releases, rel)
	}

	// High priority at full capacity gets shed (respects hard ceiling).
	result, _ := s.Acquire(context.Background(), PriorityHigh)
	if result != Shed {
		t.Fatalf("high priority at full capacity: expected Shed, got %d", result)
	}

	// Release one, high priority should be admitted directly (no queueing).
	releases[0]()
	result, rel := s.Acquire(context.Background(), PriorityHigh)
	if result != Admitted {
		t.Fatalf("high priority with capacity: expected Admitted, got %d", result)
	}
	rel()

	for _, r := range releases[1:] {
		r()
	}
}

func TestHighPriorityNeverQueues(t *testing.T) {
	// High priority requests should never sit in the queue. They either get
	// admitted immediately or are shed.
	s := New(Config{MaxConcurrent: 2, QueueSize: 5, QueueTimeout: time.Second})

	// Fill capacity.
	var releases []func()
	for i := 0; i < 2; i++ {
		_, rel := s.Acquire(context.Background(), PriorityNormal)
		releases = append(releases, rel)
	}

	// High priority should be shed, not queued.
	result, _ := s.Acquire(context.Background(), PriorityHigh)
	if result != Shed {
		t.Fatalf("expected Shed for high priority at capacity, got %d", result)
	}
	if s.QueueLen() != 0 {
		t.Fatalf("expected 0 queued after high priority shed, got %d", s.QueueLen())
	}

	for _, r := range releases {
		r()
	}
}

func TestQueueTimeoutReturns503(t *testing.T) {
	s := New(Config{MaxConcurrent: 1, QueueSize: 5, QueueTimeout: 100 * time.Millisecond})

	// Fill capacity.
	_, rel := s.Acquire(context.Background(), PriorityNormal)

	// Next request should timeout.
	start := time.Now()
	result, _ := s.Acquire(context.Background(), PriorityNormal)
	elapsed := time.Since(start)

	if result != QueueTimeout {
		t.Fatalf("expected QueueTimeout, got %d", result)
	}
	if elapsed < 80*time.Millisecond {
		t.Fatalf("timeout too fast: %v", elapsed)
	}

	rel()
}

func TestContextCancellation(t *testing.T) {
	s := New(Config{MaxConcurrent: 1, QueueSize: 5, QueueTimeout: 5 * time.Second})

	// Fill capacity.
	_, rel := s.Acquire(context.Background(), PriorityNormal)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, _ := s.Acquire(ctx, PriorityNormal)
	if result != QueueTimeout {
		t.Fatalf("expected QueueTimeout on context cancel, got %d", result)
	}

	rel()
}

func TestQueueFullShed(t *testing.T) {
	s := New(Config{MaxConcurrent: 1, QueueSize: 1, QueueTimeout: 5 * time.Second})

	// Fill capacity.
	_, rel := s.Acquire(context.Background(), PriorityNormal)

	// Fill queue (one slot).
	go func() {
		s.Acquire(context.Background(), PriorityNormal)
	}()
	time.Sleep(20 * time.Millisecond)

	// Queue is full, should be shed.
	result, _ := s.Acquire(context.Background(), PriorityNormal)
	if result != Shed {
		t.Fatalf("expected Shed when queue is full, got %d", result)
	}

	rel()
}

func TestConcurrentSafety(t *testing.T) {
	s := New(Config{MaxConcurrent: 10, QueueSize: 50, QueueTimeout: 2 * time.Second})

	var wg sync.WaitGroup
	var admitted atomic.Int64
	var shed atomic.Int64

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var p Priority
			switch i % 3 {
			case 0:
				p = PriorityHigh
			case 1:
				p = PriorityNormal
			case 2:
				p = PriorityLow
			}
			result, rel := s.Acquire(context.Background(), p)
			if result == Admitted {
				admitted.Add(1)
				time.Sleep(5 * time.Millisecond)
				rel()
			} else {
				shed.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if s.Inflight() != 0 {
		t.Fatalf("expected 0 inflight after all done, got %d", s.Inflight())
	}

	t.Logf("admitted: %d, shed: %d", admitted.Load(), shed.Load())
}

func TestParsePriority(t *testing.T) {
	tests := []struct {
		input    string
		expected Priority
	}{
		{"high", PriorityHigh},
		{"normal", PriorityNormal},
		{"low", PriorityLow},
		{"", PriorityNormal},
		{"unknown", PriorityNormal},
	}
	for _, tc := range tests {
		got := ParsePriority(tc.input)
		if got != tc.expected {
			t.Errorf("ParsePriority(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

func TestDoubleRelease(t *testing.T) {
	s := New(Config{MaxConcurrent: 10, QueueSize: 5, QueueTimeout: time.Second})

	_, rel := s.Acquire(context.Background(), PriorityNormal)
	rel()
	rel() // double release should be safe

	if s.Inflight() != 0 {
		t.Fatalf("expected 0 inflight, got %d", s.Inflight())
	}
}
