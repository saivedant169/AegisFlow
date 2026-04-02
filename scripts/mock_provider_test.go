package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestMockProviderChatCompletion(t *testing.T) {
	handler := newMockProviderHandler(0)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hello benchmark"}]}`))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp types.ChatCompletionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Model != "gpt-4o-mini" {
		t.Fatalf("expected model gpt-4o-mini, got %q", resp.Model)
	}
	if len(resp.Choices) != 1 || !strings.Contains(resp.Choices[0].Message.Content, "hello benchmark") {
		t.Fatalf("unexpected response body: %+v", resp.Choices)
	}
}

func TestMockProviderModels(t *testing.T) {
	handler := newMockProviderHandler(0)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "gpt-4o-mini") {
		t.Fatalf("expected benchmark model in response, got %s", w.Body.String())
	}
}

func TestMockProviderLatency(t *testing.T) {
	handler := newMockProviderHandler(20 * time.Millisecond)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"latency"}]}`))
	w := httptest.NewRecorder()

	start := time.Now()
	handler.ServeHTTP(w, req)

	if time.Since(start) < 20*time.Millisecond {
		t.Fatalf("expected configured latency to be applied")
	}
}
