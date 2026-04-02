package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

type MockProvider struct {
	name    string
	latency time.Duration
}

func NewMockProvider(name string, latency time.Duration) *MockProvider {
	return &MockProvider{
		name:    name,
		latency: latency,
	}
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	if m.latency > 0 {
		select {
		case <-time.After(m.latency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	userMsg := ""
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			userMsg = msg.Content
		}
	}

	reply := fmt.Sprintf("This is a mock response from AegisFlow. You said: %q", userMsg)
	promptTokens := m.EstimateTokens(userMsg)
	completionTokens := m.EstimateTokens(reply)

	return &types.ChatCompletionResponse{
		ID:      fmt.Sprintf("aegis-mock-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []types.Choice{
			{
				Index:        0,
				Message:      types.Message{Role: "assistant", Content: reply},
				FinishReason: "stop",
			},
		},
		Usage: types.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}, nil
}

func (m *MockProvider) ChatCompletionStream(ctx context.Context, req *types.ChatCompletionRequest) (io.ReadCloser, error) {
	userMsg := ""
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			userMsg = msg.Content
		}
	}

	reply := fmt.Sprintf("This is a mock streaming response from AegisFlow. You said: %q", userMsg)
	words := strings.Fields(reply)
	id := fmt.Sprintf("aegis-mock-%d", time.Now().UnixNano())

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		for i, word := range words {
			select {
			case <-ctx.Done():
				return
			default:
			}

			content := word
			if i < len(words)-1 {
				content += " "
			}

			chunk := types.StreamChunk{
				ID:      id,
				Object:  "chat.completion.chunk",
				Created: time.Now().Unix(),
				Model:   req.Model,
				Choices: []types.StreamDelta{
					{
						Index: 0,
						Delta: types.Delta{Content: content},
					},
				},
			}

			data, _ := json.Marshal(chunk)
			fmt.Fprintf(pw, "data: %s\n\n", data)

			if m.latency > 0 {
				time.Sleep(m.latency / time.Duration(len(words)))
			}
		}

		// Send final chunk with finish_reason
		finalChunk := types.StreamChunk{
			ID:      id,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []types.StreamDelta{
				{
					Index:        0,
					Delta:        types.Delta{},
					FinishReason: "stop",
				},
			},
		}
		data, _ := json.Marshal(finalChunk)
		fmt.Fprintf(pw, "data: %s\n\n", data)
		fmt.Fprint(pw, "data: [DONE]\n\n")
	}()

	return pr, nil
}

func (m *MockProvider) Models(_ context.Context) ([]types.Model, error) {
	return []types.Model{
		{ID: "mock", Object: "model", Provider: m.name},
		{ID: "mock-fast", Object: "model", Provider: m.name},
	}, nil
}

func (m *MockProvider) EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}

func (m *MockProvider) Healthy(_ context.Context) bool {
	return true
}
