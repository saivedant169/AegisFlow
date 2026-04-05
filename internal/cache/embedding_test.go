package cache

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMockEmbedder(t *testing.T) {
	mock := &MockEmbedder{
		EmbedFn: func(ctx context.Context, text string) ([]float64, error) {
			return []float64{0.1, 0.2, 0.3}, nil
		},
	}

	vec, err := mock.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3 dimensions, got %d", len(vec))
	}
	if vec[0] != 0.1 {
		t.Fatalf("expected 0.1, got %f", vec[0])
	}
}

func TestCosineSimilarityIdentical(t *testing.T) {
	a := []float64{1.0, 0.0, 0.0}
	sim := CosineSimilarity(a, a)
	if sim < 0.999 {
		t.Fatalf("identical vectors should have similarity ~1.0, got %f", sim)
	}
}

func TestCosineSimilarityOrthogonal(t *testing.T) {
	a := []float64{1.0, 0.0, 0.0}
	b := []float64{0.0, 1.0, 0.0}
	sim := CosineSimilarity(a, b)
	if sim > 0.001 {
		t.Fatalf("orthogonal vectors should have similarity ~0.0, got %f", sim)
	}
}

func TestCosineSimilarityOpposite(t *testing.T) {
	a := []float64{1.0, 0.0}
	b := []float64{-1.0, 0.0}
	sim := CosineSimilarity(a, b)
	if sim > -0.999 {
		t.Fatalf("opposite vectors should have similarity ~-1.0, got %f", sim)
	}
}

func TestCosineSimilarityMismatchedLength(t *testing.T) {
	a := []float64{1.0, 0.0}
	b := []float64{1.0, 0.0, 0.0}
	sim := CosineSimilarity(a, b)
	if sim != 0.0 {
		t.Fatalf("mismatched lengths should return 0.0, got %f", sim)
	}
}

func TestCosineSimilarityZeroVector(t *testing.T) {
	a := []float64{0.0, 0.0, 0.0}
	b := []float64{1.0, 0.0, 0.0}
	sim := CosineSimilarity(a, b)
	if sim != 0.0 {
		t.Fatalf("zero vector should return 0.0, got %f", sim)
	}
}

func TestOpenAIEmbedder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing or wrong auth header")
		}

		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)
		if reqBody["model"] != "text-embedding-3-small" {
			t.Fatalf("unexpected model: %v", reqBody["model"])
		}

		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float64{0.1, 0.2, 0.3}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	embedder := NewOpenAIEmbedder(server.URL, "test-key", "text-embedding-3-small")
	vec, err := embedder.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vec) != 3 {
		t.Fatalf("expected 3 dimensions, got %d", len(vec))
	}
}

func TestOpenAIEmbedderServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	embedder := NewOpenAIEmbedder(server.URL, "test-key", "text-embedding-3-small")
	_, err := embedder.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}
