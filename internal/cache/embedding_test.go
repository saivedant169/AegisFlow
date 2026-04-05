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
