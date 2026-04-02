package provider

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

// ---------------------------------------------------------------------------
// Registry: Register + Get
// ---------------------------------------------------------------------------

func TestRegistryRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	mock := NewMockProvider("test-provider", 0)
	reg.Register(mock)

	p, err := reg.Get("test-provider")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name() != "test-provider" {
		t.Errorf("expected name test-provider, got %s", p.Name())
	}
}

// ---------------------------------------------------------------------------
// Registry: Get nonexistent returns error
// ---------------------------------------------------------------------------

func TestRegistryGetNonexistent(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Get("does-not-exist")
	if err == nil {
		t.Fatal("expected error for nonexistent provider")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

// ---------------------------------------------------------------------------
// Registry: List returns all
// ---------------------------------------------------------------------------

func TestRegistryList(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewMockProvider("a", 0))
	reg.Register(NewMockProvider("b", 0))
	reg.Register(NewMockProvider("c", 0))

	list := reg.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(list))
	}

	names := map[string]bool{}
	for _, p := range list {
		names[p.Name()] = true
	}
	for _, expected := range []string{"a", "b", "c"} {
		if !names[expected] {
			t.Errorf("missing provider %q in list", expected)
		}
	}
}

func TestRegistryListEmpty(t *testing.T) {
	reg := NewRegistry()
	list := reg.List()
	if len(list) != 0 {
		t.Errorf("expected 0 providers, got %d", len(list))
	}
}

// ---------------------------------------------------------------------------
// Registry: AllModels aggregates
// ---------------------------------------------------------------------------

func TestRegistryAllModels(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewMockProvider("p1", 0))
	reg.Register(NewMockProvider("p2", 0))

	models := reg.AllModels()
	// Each mock provider returns 2 models
	if len(models) != 4 {
		t.Fatalf("expected 4 models from 2 providers, got %d", len(models))
	}
}

func TestRegistryAllModelsEmpty(t *testing.T) {
	reg := NewRegistry()
	models := reg.AllModels()
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d", len(models))
	}
}

// ---------------------------------------------------------------------------
// Mock provider: Name returns correct name
// ---------------------------------------------------------------------------

func TestMockProviderName(t *testing.T) {
	m := NewMockProvider("my-provider", 0)
	if m.Name() != "my-provider" {
		t.Errorf("expected name my-provider, got %s", m.Name())
	}
}

// ---------------------------------------------------------------------------
// Mock provider: EstimateTokens edge cases
// ---------------------------------------------------------------------------

func TestMockProviderEstimateTokensEmpty(t *testing.T) {
	m := NewMockProvider("mock", 0)
	if m.EstimateTokens("") != 0 {
		t.Error("expected 0 tokens for empty string")
	}
}

func TestMockProviderEstimateTokensNonEmpty(t *testing.T) {
	m := NewMockProvider("mock", 0)
	tokens := m.EstimateTokens("hello world 1234")
	if tokens != len("hello world 1234")/4 {
		t.Errorf("expected %d tokens, got %d", len("hello world 1234")/4, tokens)
	}
}

// ---------------------------------------------------------------------------
// Mock provider: ChatCompletion with latency + context cancellation
// ---------------------------------------------------------------------------

func TestMockProviderChatCompletionCancelled(t *testing.T) {
	m := NewMockProvider("mock", 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req := &types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "test"}},
	}

	_, err := m.ChatCompletion(ctx, req)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// ---------------------------------------------------------------------------
// Mock provider: ChatCompletion returns valid response (no latency)
// ---------------------------------------------------------------------------

func TestMockProviderChatCompletionNoLatency(t *testing.T) {
	m := NewMockProvider("mock", 0)

	req := &types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "ping"}},
	}

	resp, err := m.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Object != "chat.completion" {
		t.Errorf("expected object chat.completion, got %s", resp.Object)
	}
	if len(resp.ID) == 0 {
		t.Error("expected non-empty ID")
	}
	if resp.Model != "mock" {
		t.Errorf("expected model mock, got %s", resp.Model)
	}
	if resp.Choices[0].Message.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", resp.Choices[0].Message.Role)
	}
}

// ---------------------------------------------------------------------------
// Mock provider: ChatCompletionStream returns readable stream (no latency)
// ---------------------------------------------------------------------------

func TestMockProviderStreamNoLatency(t *testing.T) {
	m := NewMockProvider("mock", 0)

	req := &types.ChatCompletionRequest{
		Model:    "mock",
		Stream:   true,
		Messages: []types.Message{{Role: "user", Content: "test"}},
	}

	stream, err := m.ChatCompletionStream(context.Background(), req)
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
	if !strings.Contains(body, "stop") {
		t.Error("stream should contain stop finish reason")
	}
}

// ---------------------------------------------------------------------------
// Mock provider: Models returns mock models
// ---------------------------------------------------------------------------

func TestMockProviderModelsContent(t *testing.T) {
	m := NewMockProvider("test-prov", 0)
	models, err := m.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "mock" {
		t.Errorf("expected first model ID 'mock', got %s", models[0].ID)
	}
	if models[1].ID != "mock-fast" {
		t.Errorf("expected second model ID 'mock-fast', got %s", models[1].ID)
	}
	if models[0].Provider != "test-prov" {
		t.Errorf("expected provider 'test-prov', got %s", models[0].Provider)
	}
}

// ---------------------------------------------------------------------------
// Mock provider: Healthy returns true
// ---------------------------------------------------------------------------

func TestMockProviderHealthyAlways(t *testing.T) {
	m := NewMockProvider("mock", 0)
	if !m.Healthy(nil) {
		t.Error("mock provider should always be healthy")
	}
}

// ---------------------------------------------------------------------------
// Provider interface contract through mock
// ---------------------------------------------------------------------------

func TestMockSatisfiesProviderInterface(t *testing.T) {
	var _ Provider = (*MockProvider)(nil)
}
