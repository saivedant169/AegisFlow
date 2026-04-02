package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/pkg/types"
	"github.com/alicebob/miniredis/v2"
)

func TestBuildKey(t *testing.T) {
	msgs := []types.Message{{Role: "user", Content: "hello"}}

	key1 := BuildKey("tenant-a", "gpt-4o", msgs)
	key2 := BuildKey("tenant-a", "gpt-4o", msgs)
	key3 := BuildKey("tenant-a", "gpt-4o-mini", msgs)
	key4 := BuildKey("tenant-b", "gpt-4o", msgs)

	if key1 != key2 {
		t.Error("same input should produce same key")
	}
	if key1 == key3 {
		t.Error("different model should produce different key")
	}
	if key1 == key4 {
		t.Error("different tenant should produce different key")
	}
	if len(key1) == 0 {
		t.Error("key should not be empty")
	}
}

func TestMemoryCacheHitMiss(t *testing.T) {
	c := NewMemoryCache(time.Minute, 100)

	resp := &types.ChatCompletionResponse{
		ID:    "test-1",
		Model: "mock",
		Choices: []types.Choice{
			{Index: 0, Message: types.Message{Role: "assistant", Content: "cached response"}, FinishReason: "stop"},
		},
		Usage: types.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
	}

	// Miss
	_, ok := c.Get("key-1")
	if ok {
		t.Error("expected cache miss")
	}

	// Set and hit
	c.Set("key-1", resp)
	got, ok := c.Get("key-1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.ID != "test-1" {
		t.Errorf("expected ID test-1, got %s", got.ID)
	}
	if got.Choices[0].Message.Content != "cached response" {
		t.Error("cached content mismatch")
	}
}

func TestMemoryCacheTTLExpiry(t *testing.T) {
	c := NewMemoryCache(50*time.Millisecond, 100)

	resp := &types.ChatCompletionResponse{ID: "ttl-test", Model: "mock"}
	c.Set("key-ttl", resp)

	// Should hit immediately
	_, ok := c.Get("key-ttl")
	if !ok {
		t.Error("expected cache hit before TTL")
	}

	// Wait for expiry
	time.Sleep(60 * time.Millisecond)

	_, ok = c.Get("key-ttl")
	if ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestMemoryCacheEviction(t *testing.T) {
	c := NewMemoryCache(time.Minute, 2)

	c.Set("key-a", &types.ChatCompletionResponse{ID: "a"})
	c.Set("key-b", &types.ChatCompletionResponse{ID: "b"})
	c.Set("key-c", &types.ChatCompletionResponse{ID: "c"}) // should evict oldest

	_, ok := c.Get("key-c")
	if !ok {
		t.Error("key-c should exist")
	}

	// At least one of a or b should be evicted
	_, aOk := c.Get("key-a")
	_, bOk := c.Get("key-b")
	if aOk && bOk {
		t.Error("expected at least one eviction when cache is full")
	}
}

func TestMemoryCacheDifferentKeys(t *testing.T) {
	c := NewMemoryCache(time.Minute, 100)

	c.Set("key-1", &types.ChatCompletionResponse{ID: "resp-1"})
	c.Set("key-2", &types.ChatCompletionResponse{ID: "resp-2"})

	r1, _ := c.Get("key-1")
	r2, _ := c.Get("key-2")

	if r1.ID != "resp-1" || r2.ID != "resp-2" {
		t.Error("different keys should return different responses")
	}
}

// TestBuildKeyDifferentTenants confirms that two different tenants with
// otherwise identical inputs produce distinct cache keys.
func TestBuildKeyDifferentTenants(t *testing.T) {
	msgs := []types.Message{{Role: "user", Content: "same message"}}

	keyA := BuildKey("tenant-alpha", "gpt-4o", msgs)
	keyB := BuildKey("tenant-beta", "gpt-4o", msgs)

	if keyA == keyB {
		t.Error("different tenants must produce different cache keys")
	}
}

// TestGetExpiredEntryReturnsMiss verifies that Get returns a miss for an
// entry whose TTL has elapsed (without waiting for eviction).
func TestGetExpiredEntryReturnsMiss(t *testing.T) {
	c := NewMemoryCache(1*time.Millisecond, 100)

	c.Set("expire-me", &types.ChatCompletionResponse{ID: "old"})
	time.Sleep(5 * time.Millisecond)

	_, ok := c.Get("expire-me")
	if ok {
		t.Error("expected miss for expired entry")
	}
}

// TestStatsReturnsCorrectCounts verifies that Stats() accurately reports
// hits, misses, size, and evictions after a series of operations.
func TestStatsReturnsCorrectCounts(t *testing.T) {
	c := NewMemoryCache(time.Minute, 2)

	resp := &types.ChatCompletionResponse{ID: "stats-test"}

	// 1 miss
	c.Get("nonexistent")

	// 2 sets, 2 hits
	c.Set("k1", resp)
	c.Set("k2", resp)
	c.Get("k1")
	c.Get("k2")

	// 1 more miss
	c.Get("k3")

	// Trigger eviction: capacity is 2, setting k3 should evict one entry.
	c.Set("k3", resp)

	stats := c.Stats()

	if stats.Hits != 2 {
		t.Errorf("expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 2 {
		t.Errorf("expected 2 misses, got %d", stats.Misses)
	}
	if stats.Size != 2 {
		t.Errorf("expected size 2, got %d", stats.Size)
	}
	if stats.MaxSize != 2 {
		t.Errorf("expected max_size 2, got %d", stats.MaxSize)
	}
	if stats.Evictions < 1 {
		t.Errorf("expected at least 1 eviction, got %d", stats.Evictions)
	}
}

// TestSetAtCapacityTriggersEviction confirms that inserting a new entry when
// the cache is at capacity results in exactly one eviction.
func TestSetAtCapacityTriggersEviction(t *testing.T) {
	c := NewMemoryCache(time.Minute, 1)

	c.Set("first", &types.ChatCompletionResponse{ID: "1"})
	c.Set("second", &types.ChatCompletionResponse{ID: "2"})

	stats := c.Stats()
	if stats.Evictions != 1 {
		t.Errorf("expected 1 eviction, got %d", stats.Evictions)
	}
	if stats.Size != 1 {
		t.Errorf("expected size 1 after eviction, got %d", stats.Size)
	}

	// The evicted key should miss; the new key should hit.
	_, ok := c.Get("first")
	if ok {
		t.Error("expected 'first' to be evicted")
	}
	got, ok := c.Get("second")
	if !ok || got.ID != "2" {
		t.Error("expected 'second' to still be present")
	}
}

// TestConcurrentGetSetSafety runs parallel reads and writes to verify that
// MemoryCache does not race (run with -race to validate).
func TestConcurrentGetSetSafety(t *testing.T) {
	c := NewMemoryCache(time.Minute, 100)
	resp := &types.ChatCompletionResponse{ID: "concurrent"}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		key := "key"
		go func() {
			defer wg.Done()
			c.Set(key, resp)
		}()
		go func() {
			defer wg.Done()
			c.Get(key)
		}()
	}
	wg.Wait()

	// If we get here without a race or panic, the test passes.
	stats := c.Stats()
	if stats.Size < 1 {
		t.Error("expected at least 1 entry after concurrent operations")
	}
}

// TestSetEvictsExpiredEntriesAtCapacity verifies that when the cache is at
// capacity, Set first evicts expired entries before falling back to evicting
// the oldest non-expired entry.
func TestSetEvictsExpiredEntriesAtCapacity(t *testing.T) {
	c := NewMemoryCache(10*time.Millisecond, 2)

	c.Set("will-expire-1", &types.ChatCompletionResponse{ID: "e1"})
	c.Set("will-expire-2", &types.ChatCompletionResponse{ID: "e2"})

	// Wait for both entries to expire.
	time.Sleep(20 * time.Millisecond)

	// Now insert a new entry — both expired entries should be evicted.
	c.Set("fresh", &types.ChatCompletionResponse{ID: "f"})

	stats := c.Stats()
	// Both expired entries evicted, then fresh added.
	if stats.Evictions < 2 {
		t.Errorf("expected at least 2 evictions of expired entries, got %d", stats.Evictions)
	}
	if stats.Size != 1 {
		t.Errorf("expected size 1 (only fresh), got %d", stats.Size)
	}
}

// TestBuildKeyEmptyMessages verifies that BuildKey works with empty messages.
func TestBuildKeyEmptyMessages(t *testing.T) {
	key := BuildKey("tenant", "model", nil)
	if key == "" {
		t.Error("key should not be empty even with nil messages")
	}
	key2 := BuildKey("tenant", "model", []types.Message{})
	if key != key2 {
		t.Error("nil and empty messages should produce the same key")
	}
}

// TestMemoryCacheOverwriteSameKey verifies that setting the same key twice
// overwrites the previous value without increasing size.
func TestMemoryCacheOverwriteSameKey(t *testing.T) {
	c := NewMemoryCache(time.Minute, 10)

	c.Set("k", &types.ChatCompletionResponse{ID: "v1"})
	c.Set("k", &types.ChatCompletionResponse{ID: "v2"})

	got, ok := c.Get("k")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.ID != "v2" {
		t.Errorf("expected v2, got %s", got.ID)
	}

	stats := c.Stats()
	if stats.Size != 1 {
		t.Errorf("expected size 1 after overwrite, got %d", stats.Size)
	}
}

// TestMemoryCacheImplementsInterface verifies MemoryCache implements Cache.
func TestMemoryCacheImplementsInterface(t *testing.T) {
	var _ Cache = (*MemoryCache)(nil)
}

// --- RedisCache tests using miniredis ---

func setupRedisCache(t *testing.T) (*RedisCache, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc, err := NewRedisCache(mr.Addr(), "", 0, time.Minute)
	if err != nil {
		t.Fatalf("failed to create redis cache: %v", err)
	}
	return rc, mr
}

func TestRedisCacheSetAndGet(t *testing.T) {
	rc, _ := setupRedisCache(t)

	resp := &types.ChatCompletionResponse{
		ID:    "redis-test-1",
		Model: "gpt-4o",
		Choices: []types.Choice{
			{Index: 0, Message: types.Message{Role: "assistant", Content: "hello from redis"}, FinishReason: "stop"},
		},
	}

	rc.Set("rk1", resp)

	got, ok := rc.Get("rk1")
	if !ok {
		t.Fatal("expected cache hit from Redis")
	}
	if got.ID != "redis-test-1" {
		t.Errorf("expected ID redis-test-1, got %s", got.ID)
	}
	if got.Choices[0].Message.Content != "hello from redis" {
		t.Error("content mismatch from Redis cache")
	}
}

func TestRedisCacheGetMiss(t *testing.T) {
	rc, _ := setupRedisCache(t)

	_, ok := rc.Get("nonexistent-key")
	if ok {
		t.Error("expected cache miss for nonexistent key")
	}
}

func TestRedisCacheStats(t *testing.T) {
	rc, _ := setupRedisCache(t)

	rc.Set("s1", &types.ChatCompletionResponse{ID: "1"})
	rc.Set("s2", &types.ChatCompletionResponse{ID: "2"})

	stats := rc.Stats()
	if stats.Size != 2 {
		t.Errorf("expected size 2, got %d", stats.Size)
	}
}

func TestRedisCacheImplementsInterface(t *testing.T) {
	var _ Cache = (*RedisCache)(nil)
}

func TestNewRedisCacheFailsOnBadAddr(t *testing.T) {
	_, err := NewRedisCache("localhost:1", "", 0, time.Minute)
	if err == nil {
		t.Error("expected error connecting to invalid Redis address")
	}
}
