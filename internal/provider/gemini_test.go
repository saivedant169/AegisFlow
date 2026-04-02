package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestGeminiChatCompletion(t *testing.T) {
	gemResp := geminiResponse{
		Candidates: []geminiCandidate{
			{
				Content:      geminiContent{Role: "model", Parts: []geminiPart{{Text: "Hello from Gemini!"}}},
				FinishReason: "STOP",
			},
		},
		UsageMetadata: &geminiUsage{PromptTokenCount: 10, CandidatesTokenCount: 5, TotalTokenCount: 15},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ":generateContent") {
			t.Errorf("expected generateContent URL, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "test-gemini-key" {
			t.Error("expected API key in query parameter")
		}

		var req geminiRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Contents) == 0 {
			t.Error("expected non-empty contents")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(gemResp)
	}))
	defer srv.Close()

	p := &GeminiProvider{
		name: "gemini-test", baseURL: srv.URL, apiKey: "test-gemini-key",
		models: []string{"gemini-2.0-flash"}, client: srv.Client(),
	}

	resp, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model: "gemini-2.0-flash", Messages: []types.Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Choices[0].Message.Content != "Hello from Gemini!" {
		t.Errorf("unexpected content: %s", resp.Choices[0].Message.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestGeminiTranslateRequest(t *testing.T) {
	p := &GeminiProvider{}
	req := &types.ChatCompletionRequest{
		Model: "gemini-2.0-flash",
		Messages: []types.Message{
			{Role: "system", Content: "You are helpful"},
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
			{Role: "user", Content: "How are you?"},
		},
	}

	gemReq := p.translateRequest(req)

	if len(gemReq.Contents) != 4 {
		t.Fatalf("expected 4 contents, got %d", len(gemReq.Contents))
	}
	// system -> user in Gemini
	if gemReq.Contents[0].Role != "user" {
		t.Errorf("expected system to be mapped to 'user', got '%s'", gemReq.Contents[0].Role)
	}
	// assistant -> model in Gemini
	if gemReq.Contents[2].Role != "model" {
		t.Errorf("expected assistant to be mapped to 'model', got '%s'", gemReq.Contents[2].Role)
	}
}

func TestGeminiModels(t *testing.T) {
	p := &GeminiProvider{name: "gemini", models: []string{"gemini-2.0-flash", "gemini-1.5-pro"}}
	models, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}
}

func TestGeminiHealthy(t *testing.T) {
	p := &GeminiProvider{apiKey: "test-key"}
	if !p.Healthy(context.Background()) {
		t.Error("should be healthy with API key")
	}

	p2 := &GeminiProvider{apiKey: ""}
	if p2.Healthy(context.Background()) {
		t.Error("should be unhealthy without API key")
	}
}
