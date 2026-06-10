package cache

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// countingEmbedder records how many times Embed is called.
func countingEmbedder(calls *int64) *MockEmbedder {
	return &MockEmbedder{
		EmbedFn: func(ctx context.Context, text string) ([]float64, error) {
			atomic.AddInt64(calls, 1)
			return []float64{1, 0, 0}, nil
		},
		DimCount: 3,
	}
}

// A miss followed by a store must embed the request exactly once: GetSemantic
// computes the vector and StoreAsync reuses it instead of embedding again.
func TestSemanticMissReusesEmbedding(t *testing.T) {
	var calls int64
	sc := NewSemanticCache(countingEmbedder(&calls), 0.85, 100)
	defer sc.Close()

	req := makeReq("hello")
	resp := makeResp("world")

	cached, emb, ok := sc.GetSemanticWithEmbedding("t1", req)
	if ok || cached != nil {
		t.Fatal("expected a miss on an empty cache")
	}
	if emb == nil {
		t.Fatal("expected the lookup to return its computed embedding")
	}
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("expected exactly 1 embed call for the lookup, got %d", got)
	}

	sc.StoreAsync("t1", req, resp, emb)

	// The async insert should land without a second embed call.
	if !waitForSize(sc, 1, time.Second) {
		t.Fatal("async store did not insert in time")
	}
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("store reused the embedding but embedder was called %d times", got)
	}

	if _, hit := sc.GetSemantic("t1", req); !hit {
		t.Fatal("expected a hit after the async store landed")
	}
}

// With a nil embedding the worker embeds lazily — still off the request path.
func TestStoreAsyncEmbedsWhenNil(t *testing.T) {
	var calls int64
	sc := NewSemanticCache(countingEmbedder(&calls), 0.85, 100)
	defer sc.Close()

	sc.StoreAsync("t1", makeReq("hello"), makeResp("world"), nil)
	if !waitForSize(sc, 1, time.Second) {
		t.Fatal("async store with lazy embed did not insert in time")
	}
	if got := atomic.LoadInt64(&calls); got != 1 {
		t.Fatalf("expected 1 lazy embed call, got %d", got)
	}
}

func waitForSize(sc *SemanticCache, want int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if sc.Stats().Size == want {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return sc.Stats().Size == want
}
