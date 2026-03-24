package ratelimit

import (
	"testing"
	"time"
)

func TestMemoryLimiterAllows(t *testing.T) {
	limiter := NewMemoryLimiter(5, time.Minute)

	for i := 0; i < 5; i++ {
		allowed, err := limiter.Allow("tenant-1", 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	allowed, _ := limiter.Allow("tenant-1", 1)
	if allowed {
		t.Error("6th request should be denied (limit=5)")
	}
}

func TestMemoryLimiterSeparateTenants(t *testing.T) {
	limiter := NewMemoryLimiter(2, time.Minute)

	limiter.Allow("tenant-a", 1)
	limiter.Allow("tenant-a", 1)

	allowed, _ := limiter.Allow("tenant-a", 1)
	if allowed {
		t.Error("tenant-a should be rate limited")
	}

	allowed, _ = limiter.Allow("tenant-b", 1)
	if !allowed {
		t.Error("tenant-b should NOT be rate limited")
	}
}

func TestMemoryLimiterResets(t *testing.T) {
	limiter := NewMemoryLimiter(1, 50*time.Millisecond)

	allowed, _ := limiter.Allow("tenant-1", 1)
	if !allowed {
		t.Error("first request should be allowed")
	}

	allowed, _ = limiter.Allow("tenant-1", 1)
	if allowed {
		t.Error("second request should be denied")
	}

	time.Sleep(60 * time.Millisecond)

	allowed, _ = limiter.Allow("tenant-1", 1)
	if !allowed {
		t.Error("request after reset should be allowed")
	}
}

func TestMemoryLimiterCost(t *testing.T) {
	limiter := NewMemoryLimiter(10, time.Minute)

	allowed, _ := limiter.Allow("tenant-1", 7)
	if !allowed {
		t.Error("cost=7 should be allowed (limit=10)")
	}

	allowed, _ = limiter.Allow("tenant-1", 4)
	if allowed {
		t.Error("cost=4 should be denied (7+4 > 10)")
	}

	allowed, _ = limiter.Allow("tenant-1", 3)
	if !allowed {
		t.Error("cost=3 should be allowed (7+3 = 10)")
	}
}
