package provider

import (
	"context"
	"io"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

type Provider interface {
	Name() string
	ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error)
	ChatCompletionStream(ctx context.Context, req *types.ChatCompletionRequest) (io.ReadCloser, error)
	Models(ctx context.Context) ([]types.Model, error)
	EstimateTokens(text string) int
	Healthy(ctx context.Context) bool
}
