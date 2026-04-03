package ratelimit

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

// ---------------------------------------------------------------------------
// Limiter interface contract
// ---------------------------------------------------------------------------

func TestMemoryLimiterImplementsLimiter(t *testing.T) {
	var _ Limiter = (*MemoryLimiter)(nil)
}

func TestRedisLimiterImplementsLimiter(t *testing.T) {
	var _ Limiter = (*RedisLimiter)(nil)
}

// ---------------------------------------------------------------------------
// Memory limiter: cost > remaining budget
// ---------------------------------------------------------------------------

func TestMemoryLimiterCostExceedsBudget(t *testing.T) {
	limiter := NewMemoryLimiter(5, time.Minute)

	allowed, err := limiter.Allow("k", 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("cost=6 should be denied when limit=5")
	}
}

// ---------------------------------------------------------------------------
// Memory limiter: allow after window reset
// ---------------------------------------------------------------------------

func TestMemoryLimiterAllowAfterWindowReset(t *testing.T) {
	limiter := NewMemoryLimiter(3, 30*time.Millisecond)

	// Exhaust budget
	for i := 0; i < 3; i++ {
		limiter.Allow("k", 1)
	}
	allowed, _ := limiter.Allow("k", 1)
	if allowed {
		t.Fatal("should be denied before reset")
	}

	time.Sleep(40 * time.Millisecond)

	allowed, err := limiter.Allow("k", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("should be allowed after window reset")
	}
}

// ---------------------------------------------------------------------------
// Memory limiter: concurrent access safety
// ---------------------------------------------------------------------------

func TestMemoryLimiterConcurrentAccess(t *testing.T) {
	limiter := NewMemoryLimiter(1000, time.Minute)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				limiter.Allow("shared", 1)
			}
		}()
	}
	wg.Wait()

	// After 1000 requests of cost 1, the next should be denied
	allowed, _ := limiter.Allow("shared", 1)
	if allowed {
		t.Error("expected denial after 1000 concurrent cost-1 requests")
	}
}

// ---------------------------------------------------------------------------
// Memory limiter: concurrent access for multiple keys (exercises getOrCreate race path)
// ---------------------------------------------------------------------------

func TestMemoryLimiterConcurrentMultipleKeys(t *testing.T) {
	limiter := NewMemoryLimiter(100000, time.Minute)
	var wg sync.WaitGroup

	// Many goroutines creating the same fresh key simultaneously to trigger
	// the double-check locking path in getOrCreate (line 60).
	// Run multiple rounds with fresh keys to maximize the chance.
	for round := 0; round < 20; round++ {
		key := fmt.Sprintf("race-key-%d", round)
		ready := make(chan struct{})
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-ready // all goroutines start simultaneously
				limiter.Allow(key, 1)
			}()
		}
		close(ready)
		wg.Wait()
	}
}

// ---------------------------------------------------------------------------
// Memory limiter: 0 limit (edge case)
// ---------------------------------------------------------------------------

func TestMemoryLimiterZeroLimit(t *testing.T) {
	limiter := NewMemoryLimiter(0, time.Minute)

	allowed, err := limiter.Allow("k", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("limit=0 should deny any positive cost")
	}
}

func TestMemoryLimiterZeroLimitZeroCost(t *testing.T) {
	limiter := NewMemoryLimiter(0, time.Minute)

	allowed, err := limiter.Allow("k", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("cost=0 should be allowed even with limit=0")
	}
}

// ---------------------------------------------------------------------------
// Memory limiter: very large cost
// ---------------------------------------------------------------------------

func TestMemoryLimiterVeryLargeCost(t *testing.T) {
	limiter := NewMemoryLimiter(1000000, time.Minute)

	allowed, err := limiter.Allow("k", 1000001)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("cost exceeding limit should be denied")
	}

	allowed, err = limiter.Allow("k", 1000000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Error("cost equal to limit should be allowed")
	}
}

// ---------------------------------------------------------------------------
// Redis limiter: creation fails without a running Redis server
// ---------------------------------------------------------------------------

func TestNewRedisLimiterFailsWithBadAddress(t *testing.T) {
	_, err := NewRedisLimiter("localhost:1", "", 0, WithLimit(10), WithPeriod(time.Minute))
	if err == nil {
		t.Fatal("expected error when connecting to nonexistent Redis")
	}
}

// ---------------------------------------------------------------------------
// Redis limiter: functional tests using miniredis
// ---------------------------------------------------------------------------

func TestRedisLimiterAllowAndDeny(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	limiter, err := NewRedisLimiter(mr.Addr(), "", 0, WithLimit(5), WithPeriod(time.Minute))
	if err != nil {
		t.Fatalf("failed to create redis limiter: %v", err)
	}

	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow("test-key", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i)
		}
	}

	allowed, err := limiter.Allow("test-key", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("6th request should be denied (limit is 5)")
	}
}

func TestRedisLimiterSlidingWindow(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	limiter, err := NewRedisLimiter(mr.Addr(), "", 0,
		WithLimit(5), WithPeriod(time.Minute), WithMode(ModeSliding))
	if err != nil {
		t.Fatalf("failed to create redis limiter: %v", err)
	}

	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow("sw-key", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Errorf("request %d should be allowed", i)
		}
	}

	allowed, err := limiter.Allow("sw-key", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Error("6th request should be denied (sliding window limit is 5)")
	}
}

func TestRedisLimiterSlidingWindowAtomicCost(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	limiter, err := NewRedisLimiter(mr.Addr(), "", 0,
		WithLimit(10), WithPeriod(time.Minute), WithMode(ModeSliding))
	if err != nil {
		t.Fatal(err)
	}

	if ok, _ := limiter.Allow("c", 3); !ok {
		t.Fatal("cost 3 should be allowed")
	}
	if ok, _ := limiter.Allow("c", 3); !ok {
		t.Fatal("second cost 3 should be allowed")
	}
	if ok, _ := limiter.Allow("c", 3); !ok {
		t.Fatal("third cost 3 should be allowed (9 in window)")
	}
	if ok, _ := limiter.Allow("c", 3); ok {
		t.Fatal("fourth cost 3 should be denied (would exceed 10)")
	}
}

func TestRedisLimiterSeparateKeys(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	limiter, err := NewRedisLimiter(mr.Addr(), "", 0, WithLimit(2), WithPeriod(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	limiter.Allow("key-a", 2)
	limiter.Allow("key-b", 1)

	allowed, _ := limiter.Allow("key-a", 1)
	if allowed {
		t.Error("key-a should be exhausted")
	}

	allowed, _ = limiter.Allow("key-b", 1)
	if !allowed {
		t.Error("key-b should still have budget")
	}
}

func TestRedisLimiterCostGreaterThanLimit(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	limiter, err := NewRedisLimiter(mr.Addr(), "", 0, WithLimit(10), WithPeriod(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	allowed, _ := limiter.Allow("k", 11)
	if allowed {
		t.Error("cost exceeding limit should be denied")
	}
}
