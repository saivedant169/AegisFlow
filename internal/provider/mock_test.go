package provider

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestMockProviderChatCompletion(t *testing.T) {
	mock := NewMockProvider("mock", 0)

	req := &types.ChatCompletionRequest{
		Model: "mock",
		Messages: []types.Message{
			{Role: "user", Content: "Hello, AegisFlow!"},
		},
	}

	resp, err := mock.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Model != "mock" {
		t.Errorf("expected model 'mock', got '%s'", resp.Model)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got '%s'", resp.Choices[0].FinishReason)
	}
	if !strings.Contains(resp.Choices[0].Message.Content, "Hello, AegisFlow!") {
		t.Errorf("response should echo user message, got: %s", resp.Choices[0].Message.Content)
	}
	if resp.Usage.TotalTokens == 0 {
		t.Error("expected non-zero token usage")
	}
}

func TestMockProviderStream(t *testing.T) {
	mock := NewMockProvider("mock", 50*time.Millisecond)

	req := &types.ChatCompletionRequest{
		Model:  "mock",
		Stream: true,
		Messages: []types.Message{
			{Role: "user", Content: "Hi"},
		},
	}

	stream, err := mock.ChatCompletionStream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	data, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("failed to read stream: %v", err)
	}

	body := string(data)
	if !strings.Contains(body, "data: ") {
		t.Error("stream should contain SSE data lines")
	}
	if !strings.Contains(body, "[DONE]") {
		t.Error("stream should end with [DONE]")
	}
}

func TestMockProviderModels(t *testing.T) {
	mock := NewMockProvider("mock", 0)
	models, err := mock.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) < 1 {
		t.Error("expected at least 1 model")
	}
}

func TestMockProviderHealthy(t *testing.T) {
	mock := NewMockProvider("mock", 0)
	if !mock.Healthy(context.Background()) {
		t.Error("mock provider should always be healthy")
	}
}
