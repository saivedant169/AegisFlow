package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestKeyManagerRoundRobinAndHealth(t *testing.T) {
	km := newKeyManager("", []config.ProviderAPIKey{
		{Key: "key-a", Weight: 1},
		{Key: "key-b", Weight: 1},
	}, "round-robin", time.Minute)
	fixedNow := time.Now()
	km.now = func() time.Time { return fixedNow }

	if key := km.nextKey(); key == nil || key.key != "key-a" {
		t.Fatalf("expected key-a first, got %#v", key)
	}
	if key := km.nextKey(); key == nil || key.key != "key-b" {
		t.Fatalf("expected key-b second, got %#v", key)
	}

	km.reportStatus("openai", "key-a", http.StatusUnauthorized)
	if key := km.nextKey(); key == nil || key.key != "key-b" {
		t.Fatalf("expected key-b after key-a exclusion, got %#v", key)
	}

	km.reportStatus("openai", "key-b", http.StatusTooManyRequests)
	if key := km.nextKey(); key != nil {
		t.Fatalf("expected no key during cooldown, got %#v", key)
	}

	fixedNow = fixedNow.Add(2 * time.Minute)
	if key := km.nextKey(); key == nil || key.key != "key-b" {
		t.Fatalf("expected key-b after cooldown expiry, got %#v", key)
	}
}

func TestKeyManagerRandomSelection(t *testing.T) {
	km := newKeyManager("", []config.ProviderAPIKey{
		{Key: "key-a", Weight: 1},
		{Key: "key-b", Weight: 1},
	}, "random", time.Minute)
	seen := map[string]bool{}
	for i := 0; i < 20; i++ {
		key := km.nextKey()
		if key == nil {
			t.Fatal("expected available key")
		}
		seen[key.key] = true
	}
	if len(seen) != 2 {
		t.Fatalf("expected both keys to be selectable, got %v", seen)
	}
}

func TestOpenAIProviderRotatesAwayFromUnauthorizedKey(t *testing.T) {
	var authHeaders []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		if r.Header.Get("Authorization") == "Bearer bad-key" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"invalid key"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.ChatCompletionResponse{
			ID:      "chatcmpl-ok",
			Object:  "chat.completion",
			Model:   "gpt-4o",
			Choices: []types.Choice{{Index: 0, Message: types.Message{Role: "assistant", Content: "ok"}, FinishReason: "stop"}},
			Usage:   types.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		})
	}))
	defer srv.Close()

	p := &OpenAIProvider{
		name:    "openai-test",
		baseURL: srv.URL,
		client:  srv.Client(),
	}
	p.ConfigureKeys([]config.ProviderAPIKey{
		{Key: "bad-key", Weight: 1},
		{Key: "good-key", Weight: 1},
	}, "round-robin", time.Minute)

	req := &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "hello"}},
	}

	if _, err := p.ChatCompletion(context.Background(), req); err == nil {
		t.Fatal("expected first request to fail with unauthorized key")
	}
	resp, err := p.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("expected second request to succeed, got %v", err)
	}
	if resp.Choices[0].Message.Content != "ok" {
		t.Fatalf("unexpected response %q", resp.Choices[0].Message.Content)
	}
	if len(authHeaders) < 2 || authHeaders[0] != "Bearer bad-key" || authHeaders[1] != "Bearer good-key" {
		t.Fatalf("expected key rotation from bad-key to good-key, got %v", authHeaders)
	}
}
