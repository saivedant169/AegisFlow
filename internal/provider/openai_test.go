package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestOpenAIChatCompletion(t *testing.T) {
	mockResp := types.ChatCompletionResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: 1700000000,
		Model:   "gpt-4o",
		Choices: []types.Choice{
			{
				Index:        0,
				Message:      types.Message{Role: "assistant", Content: "Hello from mock OpenAI!"},
				FinishReason: "stop",
			},
		},
		Usage: types.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}

		var req types.ChatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "gpt-4o" {
			t.Errorf("expected model gpt-4o, got %s", req.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	p := &OpenAIProvider{
		name:    "openai-test",
		baseURL: srv.URL,
		apiKey:  "test-key",
		models:  []string{"gpt-4o"},
		client:  srv.Client(),
	}

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	resp, err := p.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "chatcmpl-test" {
		t.Errorf("expected id 'chatcmpl-test', got '%s'", resp.ID)
	}
	if resp.Choices[0].Message.Content != "Hello from mock OpenAI!" {
		t.Errorf("unexpected content: %s", resp.Choices[0].Message.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestOpenAIChatCompletionStream(t *testing.T) {
	sseData := `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: [DONE]

`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sseData))
	}))
	defer srv.Close()

	p := &OpenAIProvider{
		name:    "openai-test",
		baseURL: srv.URL,
		apiKey:  "test-key",
		models:  []string{"gpt-4o"},
		client:  srv.Client(),
	}

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Stream:   true,
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	stream, err := p.ChatCompletionStream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	data, _ := io.ReadAll(stream)
	body := string(data)

	if !strings.Contains(body, "data: ") {
		t.Error("stream should contain SSE data lines")
	}
	if !strings.Contains(body, "[DONE]") {
		t.Error("stream should end with [DONE]")
	}
}

func TestOpenAIProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
	}))
	defer srv.Close()

	p := &OpenAIProvider{
		name:    "openai-test",
		baseURL: srv.URL,
		apiKey:  "test-key",
		client:  srv.Client(),
	}

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}

	_, err := p.ChatCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for 429 response")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error should mention status 429: %v", err)
	}
}

func TestOpenAIModels(t *testing.T) {
	p := &OpenAIProvider{
		name:   "openai-test",
		models: []string{"gpt-4o", "gpt-4o-mini"},
	}

	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", models[0].ID)
	}
}
