package cache

import (
	"context"
	"testing"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func newTestEmbedder() *MockEmbedder {
	// Returns deterministic embeddings based on input content.
	// "hello" and "hi" get similar vectors; "goodbye" gets a different one.
	return &MockEmbedder{
		EmbedFn: func(ctx context.Context, text string) ([]float64, error) {
			switch {
			case text == "user: hello\n" || text == "user: hi there\n":
				return []float64{0.9, 0.1, 0.0}, nil
			case text == "user: goodbye\n":
				return []float64{0.0, 0.1, 0.9}, nil
			default:
				return []float64{0.5, 0.5, 0.0}, nil
			}
		},
		DimCount: 3,
	}
}

func makeReq(content string) *types.ChatCompletionRequest {
	return &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: content}},
	}
}

func makeResp(content string) *types.ChatCompletionResponse {
	return &types.ChatCompletionResponse{
		ID:    "resp-1",
		Model: "gpt-4o",
		Choices: []types.Choice{
			{Message: types.Message{Role: "assistant", Content: content}},
		},
	}
}

func TestSemanticCacheExactHit(t *testing.T) {
	sc := NewSemanticCache(newTestEmbedder(), 0.85, 100)
	req := makeReq("hello")
	resp := makeResp("world")

	sc.SetSemantic("tenant1", req, resp)
	got, ok := sc.GetSemantic("tenant1", req)
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got.ID != "resp-1" {
		t.Fatalf("expected resp-1, got %s", got.ID)
	}
}

func TestSemanticCacheSimilarHit(t *testing.T) {
	sc := NewSemanticCache(newTestEmbedder(), 0.85, 100)

	// Store "hello"
	sc.SetSemantic("tenant1", makeReq("hello"), makeResp("world"))

	// Query with "hi there" -- same embedding as "hello"
	got, ok := sc.GetSemantic("tenant1", makeReq("hi there"))
	if !ok {
		t.Fatal("expected semantic cache hit for similar query")
	}
	if got.Choices[0].Message.Content != "world" {
		t.Fatalf("expected 'world', got '%s'", got.Choices[0].Message.Content)
	}
}

func TestSemanticCacheDissimilarMiss(t *testing.T) {
	sc := NewSemanticCache(newTestEmbedder(), 0.85, 100)

	sc.SetSemantic("tenant1", makeReq("hello"), makeResp("world"))

	// "goodbye" has very different embedding
	_, ok := sc.GetSemantic("tenant1", makeReq("goodbye"))
	if ok {
		t.Fatal("expected cache miss for dissimilar query")
	}
}

func TestSemanticCacheTenantIsolation(t *testing.T) {
	sc := NewSemanticCache(newTestEmbedder(), 0.85, 100)

	sc.SetSemantic("tenant1", makeReq("hello"), makeResp("world"))

	_, ok := sc.GetSemantic("tenant2", makeReq("hello"))
	if ok {
		t.Fatal("expected miss -- different tenant should not hit")
	}
}

func TestSemanticCacheModelIsolation(t *testing.T) {
	sc := NewSemanticCache(newTestEmbedder(), 0.85, 100)

	sc.SetSemantic("tenant1", makeReq("hello"), makeResp("world"))

	req := makeReq("hello")
	req.Model = "claude-sonnet-4"
	_, ok := sc.GetSemantic("tenant1", req)
	if ok {
		t.Fatal("expected miss -- different model should not hit")
	}
}

func TestSemanticCacheEviction(t *testing.T) {
	sc := NewSemanticCache(newTestEmbedder(), 0.85, 2)

	sc.SetSemantic("t1", makeReq("hello"), makeResp("r1"))
	sc.SetSemantic("t1", makeReq("goodbye"), makeResp("r2"))
	// This should evict the oldest entry
	sc.SetSemantic("t1", makeReq("default"), makeResp("r3"))

	// "hello" was first in, should be evicted
	_, ok := sc.GetSemantic("t1", makeReq("hello"))
	if ok {
		t.Fatal("expected eviction of oldest entry")
	}
}

func TestSemanticCacheImplementsCacheInterface(t *testing.T) {
	sc := NewSemanticCache(newTestEmbedder(), 0.85, 100)
	var _ Cache = sc // compile-time check
}

func TestSemanticCacheStats(t *testing.T) {
	sc := NewSemanticCache(newTestEmbedder(), 0.85, 100)

	sc.SetSemantic("t1", makeReq("hello"), makeResp("world"))
	sc.GetSemantic("t1", makeReq("hello"))   // hit
	sc.GetSemantic("t1", makeReq("goodbye")) // miss

	stats := sc.Stats()
	if stats.Hits != 1 {
		t.Fatalf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.Size != 1 {
		t.Fatalf("expected size 1, got %d", stats.Size)
	}
	if stats.MaxSize != 100 {
		t.Fatalf("expected max_size 100, got %d", stats.MaxSize)
	}
}
