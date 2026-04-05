package cache

import (
	"context"
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
