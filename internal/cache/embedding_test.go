package cache

import (
	"context"
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
