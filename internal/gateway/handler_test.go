package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aegisflow/aegisflow/internal/config"
	"github.com/aegisflow/aegisflow/internal/policy"
	"github.com/aegisflow/aegisflow/internal/provider"
	"github.com/aegisflow/aegisflow/internal/router"
	"github.com/aegisflow/aegisflow/internal/usage"
	"github.com/aegisflow/aegisflow/pkg/types"
)

func setupTestHandler() *Handler {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())
	return NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, nil)
}

func TestChatCompletionSuccess(t *testing.T) {
	h := setupTestHandler()

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp types.ChatCompletionResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got '%s'", resp.Choices[0].FinishReason)
	}
}

func TestChatCompletionMissingModel(t *testing.T) {
	h := setupTestHandler()

	reqBody := types.ChatCompletionRequest{
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatCompletionMissingMessages(t *testing.T) {
	h := setupTestHandler()

	reqBody := types.ChatCompletionRequest{Model: "mock"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatCompletionStream(t *testing.T) {
	h := setupTestHandler()

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
		Stream:   true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", contentType)
	}
}

func TestListModels(t *testing.T) {
	h := setupTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()

	h.ListModels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp types.ModelList
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Object != "list" {
		t.Errorf("expected object 'list', got '%s'", resp.Object)
	}
	if len(resp.Data) == 0 {
		t.Error("expected at least 1 model")
	}
}

func TestInvalidJSON(t *testing.T) {
	h := setupTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
