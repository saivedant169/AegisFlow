package cache

import (
	"context"
	"math"
)

// Embedder converts text into a vector embedding.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float64, error)
	Dimensions() int
}

// MockEmbedder is a test double for Embedder.
type MockEmbedder struct {
	EmbedFn  func(ctx context.Context, text string) ([]float64, error)
	DimCount int
}

func (m *MockEmbedder) Embed(ctx context.Context, text string) ([]float64, error) {
	return m.EmbedFn(ctx, text)
}

func (m *MockEmbedder) Dimensions() int {
	if m.DimCount > 0 {
		return m.DimCount
	}
	return 3
}

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns 0.0 for mismatched lengths or zero-magnitude vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
