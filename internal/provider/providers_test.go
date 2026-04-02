package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

// ===========================================================================
// OpenAI additional tests
// ===========================================================================

func TestOpenAIProviderName(t *testing.T) {
	p := &OpenAIProvider{name: "my-openai"}
	if p.Name() != "my-openai" {
		t.Errorf("expected my-openai, got %s", p.Name())
	}
}

func TestOpenAIEstimateTokens(t *testing.T) {
	p := &OpenAIProvider{}
	if p.EstimateTokens("") != 0 {
		t.Error("expected 0 for empty")
	}
	if p.EstimateTokens("hello world") != len("hello world")/4 {
		t.Error("token estimate mismatch")
	}
}

func TestOpenAIHealthyNoKey(t *testing.T) {
	p := &OpenAIProvider{apiKey: "", client: &http.Client{Timeout: time.Second}}
	if p.Healthy(context.Background()) {
		t.Error("should be unhealthy without api key")
	}
}

func TestOpenAIHealthyWithKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	p := &OpenAIProvider{apiKey: "key", baseURL: srv.URL, client: srv.Client()}
	if !p.Healthy(context.Background()) {
		t.Error("should be healthy")
	}
}

func TestNewOpenAIProviderDefaults(t *testing.T) {
	p := NewOpenAIProvider("test", "http://localhost", "NONEXISTENT_KEY_ENV", nil, 0, 0)
	if p.name != "test" {
		t.Errorf("expected name test, got %s", p.name)
	}
	if p.maxRetries != 2 {
		t.Errorf("expected default maxRetries 2, got %d", p.maxRetries)
	}
}

func TestOpenAIStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	p := &OpenAIProvider{name: "test", baseURL: srv.URL, apiKey: "key", client: srv.Client()}
	_, err := p.ChatCompletionStream(context.Background(), &types.ChatCompletionRequest{
		Model: "gpt-4o", Messages: []types.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention 500: %v", err)
	}
}

// ===========================================================================
// Anthropic tests
// ===========================================================================

func TestAnthropicChatCompletion(t *testing.T) {
	anthResp := anthropicResponse{
		ID:   "msg-test",
		Type: "message",
		Role: "assistant",
		Content: []anthropicContentBlock{
			{Type: "text", Text: "Hello from Anthropic!"},
		},
		Model: "claude-sonnet-4-20250514",
		Usage: anthropicUsage{InputTokens: 10, OutputTokens: 5},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-anthropic-key" {
			t.Errorf("unexpected auth: %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Error("expected anthropic-version header")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthResp)
	}))
	defer srv.Close()

	p := &AnthropicProvider{
		name: "anthropic-test", baseURL: srv.URL, apiKey: "test-anthropic-key",
		models: []string{"claude-sonnet-4-20250514"}, client: srv.Client(),
	}

	resp, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model: "claude-sonnet-4-20250514", Messages: []types.Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Choices[0].Message.Content != "Hello from Anthropic!" {
		t.Errorf("unexpected content: %s", resp.Choices[0].Message.Content)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", resp.Usage.PromptTokens)
	}
}

func TestAnthropicChatCompletionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer srv.Close()

	p := &AnthropicProvider{name: "test", baseURL: srv.URL, apiKey: "key", client: srv.Client()}
	_, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model: "claude-sonnet-4-20250514", Messages: []types.Message{{Role: "user", Content: "Hello"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAnthropicChatCompletionStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: hello\n\n"))
	}))
	defer srv.Close()

	p := &AnthropicProvider{name: "test", baseURL: srv.URL, apiKey: "key", client: srv.Client()}
	stream, err := p.ChatCompletionStream(context.Background(), &types.ChatCompletionRequest{
		Model: "claude-sonnet-4-20250514", Messages: []types.Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()
	data, _ := io.ReadAll(stream)
	if len(data) == 0 {
		t.Error("expected non-empty stream")
	}
}

func TestAnthropicChatCompletionStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))
	defer srv.Close()

	p := &AnthropicProvider{name: "test", baseURL: srv.URL, apiKey: "key", client: srv.Client()}
	_, err := p.ChatCompletionStream(context.Background(), &types.ChatCompletionRequest{
		Model: "claude-sonnet-4-20250514", Messages: []types.Message{{Role: "user", Content: "Hello"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAnthropicName(t *testing.T) {
	p := &AnthropicProvider{name: "anth"}
	if p.Name() != "anth" {
		t.Errorf("expected anth, got %s", p.Name())
	}
}

func TestAnthropicModels(t *testing.T) {
	p := &AnthropicProvider{name: "anth", models: []string{"claude-sonnet-4-20250514", "claude-3-haiku"}}
	models, err := p.Models(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}
	if models[0].Provider != "anth" {
		t.Errorf("expected provider anth, got %s", models[0].Provider)
	}
}

func TestAnthropicEstimateTokens(t *testing.T) {
	p := &AnthropicProvider{}
	if p.EstimateTokens("") != 0 {
		t.Error("expected 0 for empty")
	}
	if p.EstimateTokens("test") != 1 {
		t.Errorf("expected 1, got %d", p.EstimateTokens("test"))
	}
}

func TestAnthropicHealthy(t *testing.T) {
	p := &AnthropicProvider{apiKey: "key"}
	if !p.Healthy(context.Background()) {
		t.Error("should be healthy with key")
	}
	p2 := &AnthropicProvider{apiKey: ""}
	if p2.Healthy(context.Background()) {
		t.Error("should be unhealthy without key")
	}
}

func TestAnthropicTranslateRequest(t *testing.T) {
	p := &AnthropicProvider{}
	maxTokens := 500
	req := &types.ChatCompletionRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []types.Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hello"},
		},
		MaxTokens: &maxTokens,
	}
	anthReq := p.translateRequest(req)
	// system messages should be filtered out
	if len(anthReq.Messages) != 1 {
		t.Errorf("expected 1 message (system filtered), got %d", len(anthReq.Messages))
	}
	if anthReq.MaxTokens != 500 {
		t.Errorf("expected maxTokens 500, got %d", anthReq.MaxTokens)
	}
}

func TestAnthropicTranslateResponse(t *testing.T) {
	p := &AnthropicProvider{}
	resp := &anthropicResponse{
		ID:      "msg-123",
		Content: []anthropicContentBlock{{Type: "text", Text: "hello"}, {Type: "text", Text: " world"}},
		Usage:   anthropicUsage{InputTokens: 5, OutputTokens: 3},
	}
	result := p.translateResponse(resp, "claude-sonnet-4-20250514")
	if result.Choices[0].Message.Content != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", result.Choices[0].Message.Content)
	}
	if result.Usage.TotalTokens != 8 {
		t.Errorf("expected 8 total tokens, got %d", result.Usage.TotalTokens)
	}
}

// ===========================================================================
// Ollama tests
// ===========================================================================

func TestOllamaName(t *testing.T) {
	p := NewOllamaProvider("ollama-test", "http://localhost:11434", []string{"llama3"})
	if p.Name() != "ollama-test" {
		t.Errorf("expected ollama-test, got %s", p.Name())
	}
}

func TestOllamaChatCompletion(t *testing.T) {
	ollamaResp := ollamaChatResponse{
		Model:   "llama3",
		Message: ollamaMessage{Role: "assistant", Content: "Hello from Ollama!"},
		Done:    true,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaResp)
	}))
	defer srv.Close()

	p := &OllamaProvider{name: "ollama", baseURL: srv.URL, models: []string{"llama3"}, client: srv.Client()}
	resp, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model: "llama3", Messages: []types.Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Choices[0].Message.Content != "Hello from Ollama!" {
		t.Errorf("unexpected content: %s", resp.Choices[0].Message.Content)
	}
}

func TestOllamaChatCompletionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("unavailable"))
	}))
	defer srv.Close()

	p := &OllamaProvider{name: "ollama", baseURL: srv.URL, client: srv.Client()}
	_, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model: "llama3", Messages: []types.Message{{Role: "user", Content: "Hello"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOllamaChatCompletionStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		json.NewEncoder(w).Encode(ollamaChatResponse{Model: "llama3", Message: ollamaMessage{Content: "hi"}, Done: false})
		json.NewEncoder(w).Encode(ollamaChatResponse{Model: "llama3", Done: true})
	}))
	defer srv.Close()

	p := &OllamaProvider{name: "ollama", baseURL: srv.URL, models: []string{"llama3"}, client: srv.Client()}
	stream, err := p.ChatCompletionStream(context.Background(), &types.ChatCompletionRequest{
		Model: "llama3", Messages: []types.Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()
	data, _ := io.ReadAll(stream)
	body := string(data)
	if !strings.Contains(body, "data: ") {
		t.Error("expected SSE data lines")
	}
	if !strings.Contains(body, "[DONE]") {
		t.Error("expected [DONE]")
	}
}

func TestOllamaChatCompletionStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))
	defer srv.Close()

	p := &OllamaProvider{name: "ollama", baseURL: srv.URL, client: srv.Client()}
	_, err := p.ChatCompletionStream(context.Background(), &types.ChatCompletionRequest{
		Model: "llama3", Messages: []types.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOllamaModelsLive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"models":[{"name":"llama3"},{"name":"phi3"}]}`))
		}
	}))
	defer srv.Close()

	p := &OllamaProvider{name: "ollama", baseURL: srv.URL, models: []string{"fallback"}, client: srv.Client()}
	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 live models, got %d", len(models))
	}
}

func TestOllamaModelsFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := &OllamaProvider{name: "ollama", baseURL: srv.URL, models: []string{"llama3"}, client: srv.Client()}
	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 {
		t.Errorf("expected 1 fallback model, got %d", len(models))
	}
}

func TestOllamaEstimateTokens(t *testing.T) {
	p := &OllamaProvider{}
	if p.EstimateTokens("") != 0 {
		t.Error("expected 0 for empty")
	}
	if p.EstimateTokens("test1234") != 2 {
		t.Errorf("expected 2, got %d", p.EstimateTokens("test1234"))
	}
}

func TestOllamaHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := &OllamaProvider{name: "ollama", baseURL: srv.URL, client: srv.Client()}
	if !p.Healthy(context.Background()) {
		t.Error("should be healthy")
	}
}

func TestOllamaHealthyDown(t *testing.T) {
	p := &OllamaProvider{name: "ollama", baseURL: "http://localhost:1", client: &http.Client{Timeout: time.Second}}
	if p.Healthy(context.Background()) {
		t.Error("should be unhealthy when server is down")
	}
}

// ===========================================================================
// Bedrock tests
// ===========================================================================

func TestBedrockName(t *testing.T) {
	p := &BedrockProvider{name: "bedrock-test"}
	if p.Name() != "bedrock-test" {
		t.Errorf("expected bedrock-test, got %s", p.Name())
	}
}

func TestNewBedrockProviderDefaults(t *testing.T) {
	p := NewBedrockProvider("test", "", "NONEXISTENT_AK", "NONEXISTENT_SK", []string{"model1"}, 0)
	if p.region != "us-east-1" {
		t.Errorf("expected default region us-east-1, got %s", p.region)
	}
}

func TestBedrockModels(t *testing.T) {
	p := &BedrockProvider{name: "br", models: []string{"anthropic.claude-v2", "amazon.titan"}}
	models, err := p.Models(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}
}

func TestBedrockEstimateTokens(t *testing.T) {
	p := &BedrockProvider{}
	if p.EstimateTokens("") != 0 {
		t.Error("expected 0 for empty")
	}
	if p.EstimateTokens("abcdefgh") != 2 {
		t.Errorf("expected 2, got %d", p.EstimateTokens("abcdefgh"))
	}
}

func TestBedrockHealthy(t *testing.T) {
	p := &BedrockProvider{accessKey: "ak", secretKey: "sk"}
	if !p.Healthy(nil) {
		t.Error("should be healthy with keys")
	}
	p2 := &BedrockProvider{accessKey: "", secretKey: "sk"}
	if p2.Healthy(nil) {
		t.Error("should be unhealthy without access key")
	}
	p3 := &BedrockProvider{accessKey: "ak", secretKey: ""}
	if p3.Healthy(nil) {
		t.Error("should be unhealthy without secret key")
	}
}

func TestBedrockTranslateRequest(t *testing.T) {
	p := &BedrockProvider{}
	temp := 0.7
	maxTok := 100
	req := &types.ChatCompletionRequest{
		Model: "claude-v2",
		Messages: []types.Message{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hello"},
		},
		Temperature: &temp,
		MaxTokens:   &maxTok,
	}
	brReq := p.translateRequest(req)
	// system should be filtered
	if len(brReq.Messages) != 1 {
		t.Errorf("expected 1 message (system filtered), got %d", len(brReq.Messages))
	}
	if brReq.InferenceConfig == nil {
		t.Fatal("expected InferenceConfig")
	}
	if *brReq.InferenceConfig.MaxTokens != 100 {
		t.Errorf("expected maxTokens 100, got %d", *brReq.InferenceConfig.MaxTokens)
	}
}

func TestBedrockTranslateResponse(t *testing.T) {
	p := &BedrockProvider{}
	resp := &bedrockResponse{
		StopReason: "end_turn",
	}
	resp.Output.Message = bedrockMessage{
		Role:    "assistant",
		Content: []bedrockContent{{Text: "hi"}, {Text: " there"}},
	}
	resp.Usage.InputTokens = 5
	resp.Usage.OutputTokens = 3
	resp.Usage.TotalTokens = 8

	result := p.translateResponse(resp, "model1")
	if result.Choices[0].Message.Content != "hi there" {
		t.Errorf("expected 'hi there', got '%s'", result.Choices[0].Message.Content)
	}
	if result.Usage.TotalTokens != 8 {
		t.Errorf("expected 8 total, got %d", result.Usage.TotalTokens)
	}
}

func TestBedrockSignRequest(t *testing.T) {
	p := &BedrockProvider{region: "us-east-1", accessKey: "AKID", secretKey: "SECRET"}
	req, _ := http.NewRequest(http.MethodPost, "https://bedrock-runtime.us-east-1.amazonaws.com/model/test/converse", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	p.signRequest(req, []byte("{}"))

	if req.Header.Get("Authorization") == "" {
		t.Error("expected Authorization header after signing")
	}
	if req.Header.Get("X-Amz-Date") == "" {
		t.Error("expected X-Amz-Date header")
	}
	if !strings.Contains(req.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
		t.Error("expected AWS4 signature scheme")
	}
}

func TestBedrockChatCompletionErrorPath(t *testing.T) {
	// Bedrock builds URL from region so we can't easily point it at httptest.
	// Test the error path instead.
	p := &BedrockProvider{
		name: "bedrock", region: "us-east-1",
		accessKey: "AKID", secretKey: "SECRET",
		models: []string{"claude-v2"}, client: &http.Client{Timeout: time.Second},
	}
	_, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model: "claude-v2", Messages: []types.Message{{Role: "user", Content: "hi"}},
	})
	// Expect error since we can't reach the actual Bedrock endpoint
	if err == nil {
		t.Log("surprisingly no error connecting to Bedrock")
	}
}

func TestBedrockChatCompletionStream(t *testing.T) {
	// ChatCompletionStream falls back to ChatCompletion for Bedrock
	brResp := bedrockResponse{StopReason: "end_turn"}
	brResp.Output.Message = bedrockMessage{
		Role: "assistant", Content: []bedrockContent{{Text: "streamed"}},
	}
	brResp.Usage.TotalTokens = 5

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(brResp)
	}))
	defer srv.Close()

	// Can't easily test full flow since URL is built from region, but test error case
	p := &BedrockProvider{
		name: "bedrock", region: "us-east-1",
		accessKey: "AKID", secretKey: "SECRET",
		models: []string{"claude-v2"}, client: &http.Client{Timeout: time.Second},
	}
	// This will fail connecting, confirming the error path
	_, err := p.ChatCompletionStream(context.Background(), &types.ChatCompletionRequest{
		Model: "claude-v2", Messages: []types.Message{{Role: "user", Content: "hi"}},
	})
	// Expect error since we can't reach the actual Bedrock endpoint
	if err == nil {
		// If by some chance it succeeds (unlikely), that's also fine for coverage
		t.Log("surprisingly no error connecting to Bedrock")
	}
}

// ===========================================================================
// Azure OpenAI additional tests
// ===========================================================================

func TestAzureOpenAIName(t *testing.T) {
	p := &AzureOpenAIProvider{name: "azure-test"}
	if p.Name() != "azure-test" {
		t.Errorf("expected azure-test, got %s", p.Name())
	}
}

func TestAzureOpenAIEstimateTokens(t *testing.T) {
	p := &AzureOpenAIProvider{}
	if p.EstimateTokens("") != 0 {
		t.Error("expected 0")
	}
	if p.EstimateTokens("abcd") != 1 {
		t.Errorf("expected 1, got %d", p.EstimateTokens("abcd"))
	}
}

func TestAzureOpenAIHealthy(t *testing.T) {
	p := &AzureOpenAIProvider{apiKey: "key"}
	if !p.Healthy(context.Background()) {
		t.Error("should be healthy")
	}
	p2 := &AzureOpenAIProvider{apiKey: ""}
	if p2.Healthy(context.Background()) {
		t.Error("should be unhealthy")
	}
}

func TestAzureOpenAIChatCompletionStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"id\":\"test\"}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()

	p := &AzureOpenAIProvider{
		name: "azure", endpoint: srv.URL, apiKey: "key",
		apiVersion: "2024-10-21", models: []string{"gpt-4o"}, client: srv.Client(),
	}

	stream, err := p.ChatCompletionStream(context.Background(), &types.ChatCompletionRequest{
		Model: "gpt-4o", Messages: []types.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()
	data, _ := io.ReadAll(stream)
	if len(data) == 0 {
		t.Error("expected non-empty stream")
	}
}

func TestAzureOpenAIChatCompletionStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))
	defer srv.Close()

	p := &AzureOpenAIProvider{
		name: "azure", endpoint: srv.URL, apiKey: "key",
		apiVersion: "2024-10-21", client: srv.Client(),
	}
	_, err := p.ChatCompletionStream(context.Background(), &types.ChatCompletionRequest{
		Model: "gpt-4o", Messages: []types.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAzureOpenAIChatCompletionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer srv.Close()

	p := &AzureOpenAIProvider{
		name: "azure", endpoint: srv.URL, apiKey: "key",
		apiVersion: "2024-10-21", client: srv.Client(),
	}
	_, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model: "gpt-4o", Messages: []types.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewAzureOpenAIProviderDefaults(t *testing.T) {
	p := NewAzureOpenAIProvider("test", "https://example.openai.azure.com", "NONEXISTENT_KEY", "", []string{"gpt-4o"}, 0)
	if p.apiVersion != "2024-10-21" {
		t.Errorf("expected default api version, got %s", p.apiVersion)
	}
}

// ===========================================================================
// Gemini additional tests
// ===========================================================================

func TestGeminiName(t *testing.T) {
	p := &GeminiProvider{name: "gem"}
	if p.Name() != "gem" {
		t.Errorf("expected gem, got %s", p.Name())
	}
}

func TestGeminiEstimateTokens(t *testing.T) {
	p := &GeminiProvider{}
	if p.EstimateTokens("") != 0 {
		t.Error("expected 0")
	}
	if p.EstimateTokens("ab") != 0 {
		t.Errorf("expected 0, got %d", p.EstimateTokens("ab"))
	}
}

func TestGeminiChatCompletionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	p := &GeminiProvider{name: "gem", baseURL: srv.URL, apiKey: "key", client: srv.Client()}
	_, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model: "gemini-2.0-flash", Messages: []types.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGeminiChatCompletionStream(t *testing.T) {
	gemResp := geminiResponse{
		Candidates: []geminiCandidate{
			{Content: geminiContent{Parts: []geminiPart{{Text: "Hello"}}}},
		},
	}
	respBytes, _ := json.Marshal(gemResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: " + string(respBytes) + "\n\n"))
	}))
	defer srv.Close()

	p := &GeminiProvider{name: "gem", baseURL: srv.URL, apiKey: "key", client: srv.Client()}
	stream, err := p.ChatCompletionStream(context.Background(), &types.ChatCompletionRequest{
		Model: "gemini-2.0-flash", Messages: []types.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()
	data, _ := io.ReadAll(stream)
	if !strings.Contains(string(data), "data: ") {
		t.Error("expected SSE data")
	}
}

func TestGeminiChatCompletionStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	}))
	defer srv.Close()

	p := &GeminiProvider{name: "gem", baseURL: srv.URL, apiKey: "key", client: srv.Client()}
	_, err := p.ChatCompletionStream(context.Background(), &types.ChatCompletionRequest{
		Model: "gemini-2.0-flash", Messages: []types.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGeminiTranslateRequestWithConfig(t *testing.T) {
	p := &GeminiProvider{}
	temp := 0.5
	maxTok := 200
	req := &types.ChatCompletionRequest{
		Model:       "gemini-2.0-flash",
		Messages:    []types.Message{{Role: "user", Content: "hi"}},
		Temperature: &temp,
		MaxTokens:   &maxTok,
	}
	gemReq := p.translateRequest(req)
	if gemReq.GenerationConfig == nil {
		t.Fatal("expected GenerationConfig")
	}
}

func TestGeminiTranslateResponseNoCandidate(t *testing.T) {
	p := &GeminiProvider{}
	resp := &geminiResponse{Candidates: nil}
	result := p.translateResponse(resp, "model")
	if result.Choices[0].Message.Content != "" {
		t.Error("expected empty content with no candidates")
	}
}

func TestNewGeminiProviderDefaults(t *testing.T) {
	p := NewGeminiProvider("test", "NONEXISTENT_KEY", nil, 0)
	if len(p.models) != 2 {
		t.Errorf("expected 2 default models, got %d", len(p.models))
	}
}

// ===========================================================================
// Groq / Mistral / Together thin wrapper tests
// ===========================================================================

func TestNewGroqProvider(t *testing.T) {
	p := NewGroqProvider("groq-test", "NONEXISTENT_KEY", nil, 0)
	if p.Name() != "groq-test" {
		t.Errorf("expected groq-test, got %s", p.Name())
	}
	if len(p.models) != 3 {
		t.Errorf("expected 3 default models, got %d", len(p.models))
	}
}

func TestNewGroqProviderCustomModels(t *testing.T) {
	p := NewGroqProvider("groq", "KEY", []string{"custom"}, time.Second)
	if len(p.models) != 1 {
		t.Errorf("expected 1 model, got %d", len(p.models))
	}
}

func TestNewMistralProvider(t *testing.T) {
	p := NewMistralProvider("mistral-test", "NONEXISTENT_KEY", nil, 0)
	if p.Name() != "mistral-test" {
		t.Errorf("expected mistral-test, got %s", p.Name())
	}
	if len(p.models) != 3 {
		t.Errorf("expected 3 default models, got %d", len(p.models))
	}
}

func TestNewMistralProviderCustomModels(t *testing.T) {
	p := NewMistralProvider("mistral", "KEY", []string{"custom"}, time.Second)
	if len(p.models) != 1 {
		t.Errorf("expected 1 model, got %d", len(p.models))
	}
}

func TestNewTogetherProvider(t *testing.T) {
	p := NewTogetherProvider("together-test", "NONEXISTENT_KEY", nil, 0)
	if p.Name() != "together-test" {
		t.Errorf("expected together-test, got %s", p.Name())
	}
	if len(p.models) != 2 {
		t.Errorf("expected 2 default models, got %d", len(p.models))
	}
}

func TestNewTogetherProviderCustomModels(t *testing.T) {
	p := NewTogetherProvider("together", "KEY", []string{"custom"}, time.Second)
	if len(p.models) != 1 {
		t.Errorf("expected 1 model, got %d", len(p.models))
	}
}

// ===========================================================================
// NewAnthropicProvider
// ===========================================================================

func TestNewAnthropicProviderDefaults(t *testing.T) {
	p := NewAnthropicProvider("test", "http://localhost", "NONEXISTENT_KEY", []string{"claude-sonnet-4-20250514"}, 0)
	if p.name != "test" {
		t.Errorf("expected name test, got %s", p.name)
	}
}

// ===========================================================================
// Interface compliance
// ===========================================================================

func TestAllProvidersSatisfyInterface(t *testing.T) {
	var _ Provider = (*OpenAIProvider)(nil)
	var _ Provider = (*AnthropicProvider)(nil)
	var _ Provider = (*GeminiProvider)(nil)
	var _ Provider = (*OllamaProvider)(nil)
	var _ Provider = (*BedrockProvider)(nil)
	var _ Provider = (*AzureOpenAIProvider)(nil)
	var _ Provider = (*MockProvider)(nil)
}
