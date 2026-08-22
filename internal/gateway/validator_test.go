package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/router"
	"github.com/saivedant169/AegisFlow/internal/usage"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestValidation_ValidRequest(t *testing.T) {
	reqBody := `{"model": "gpt-4", "messages": [{"role": "user", "content": "hi"}]}`
	msg, param, code, err := validateRequest([]byte(reqBody), chatCompletionSchema)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if msg != "" || param != "" || code != "" {
		t.Errorf("expected empty returns, got msg=%s param=%s code=%s", msg, param, code)
	}
}

func TestValidation_MissingModel(t *testing.T) {
	reqBody := `{"messages": [{"role": "user", "content": "hi"}]}`
	msg, param, code, err := validateRequest([]byte(reqBody), chatCompletionSchema)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if param != "model" {
		t.Errorf("expected param 'model', got '%s'", param)
	}
	if msg == "" {
		t.Errorf("expected message, got empty")
	}
	if code != "invalid_request_error" {
		t.Errorf("expected code 'invalid_request_error', got '%s'", code)
	}
}

func TestValidation_WrongTypeMessages(t *testing.T) {
	reqBody := `{"model": "gpt-4", "messages": "not an array"}`
	msg, param, code, err := validateRequest([]byte(reqBody), chatCompletionSchema)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if param != "messages" {
		t.Errorf("expected param 'messages', got '%s'", param)
	}
	if code != "invalid_type" {
		t.Errorf("expected code 'invalid_type', got '%s'", code)
	}
	if !strings.Contains(msg, "expected array") {
		t.Errorf("expected 'expected array' in message, got '%s'", msg)
	}
}

func TestHandler_RequestValidation(t *testing.T) {
	registry := provider.NewRegistry()
	rt := router.NewRouter(nil, registry)
	h := NewHandler(registry, rt, nil, usage.NewTracker(usage.NewStore()), nil, nil, nil, nil, 0, nil, nil)
	h.SetRequestValidation(true)

	reqBody := `{"model": "gpt-4", "messages": "not an array"}`
	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", res.StatusCode)
	}

	var errResp types.ErrorResponse
	json.NewDecoder(res.Body).Decode(&errResp)

	if errResp.Error.Param != "messages" {
		t.Errorf("expected param 'messages', got '%s'", errResp.Error.Param)
	}
}
