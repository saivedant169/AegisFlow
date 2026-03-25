package cache

import (
	"testing"
	"time"

	"github.com/aegisflow/aegisflow/pkg/types"
)

func TestBuildKey(t *testing.T) {
	msgs := []types.Message{{Role: "user", Content: "hello"}}

	key1 := BuildKey("gpt-4o", msgs)
	key2 := BuildKey("gpt-4o", msgs)
	key3 := BuildKey("gpt-4o-mini", msgs)

	if key1 != key2 {
		t.Error("same input should produce same key")
	}
	if key1 == key3 {
		t.Error("different model should produce different key")
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
