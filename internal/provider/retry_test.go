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

func TestRetryPolicyDelayUsesRetryAfter(t *testing.T) {
	policy := newRetryPolicy(config.RetryConfig{MaxAttempts: 3, RetryableStatusCodes: []int{429}})
	header := http.Header{}
	header.Set("Retry-After", "1")
	if got := policy.delayForAttempt(1, header); got != time.Second {
		t.Fatalf("expected 1s delay, got %v", got)
	}
}

func TestRetryPolicyJitterStaysWithinBounds(t *testing.T) {
	policy := newRetryPolicy(config.RetryConfig{
		MaxAttempts:       3,
		InitialBackoff:    200 * time.Millisecond,
		MaxBackoff:        5 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            true,
	})
	for i := 0; i < 20; i++ {
		delay := policy.delayForAttempt(2, http.Header{})
		if delay < 0 || delay > 400*time.Millisecond {
			t.Fatalf("expected jittered delay within [0, 400ms], got %v", delay)
		}
	}
}

func TestOpenAIProviderRetriesWithRetryAfter(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.ChatCompletionResponse{
			ID:      "chatcmpl-test",
			Object:  "chat.completion",
			Model:   "gpt-4o",
			Choices: []types.Choice{{Index: 0, Message: types.Message{Role: "assistant", Content: "retry success"}, FinishReason: "stop"}},
			Usage:   types.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		})
	}))
	defer srv.Close()

	p := &OpenAIProvider{
		name:    "openai-test",
		baseURL: srv.URL,
		keys:    NewKeyRotator([]string{"test-key"}, "round-robin", 0),
		client:  srv.Client(),
	}
	p.ConfigureRetry(config.RetryConfig{MaxAttempts: 2, RetryableStatusCodes: []int{429}})
	var slept []time.Duration
	p.sleep = func(d time.Duration) { slept = append(slept, d) }

	resp, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model:    "gpt-4o",
		Messages: []types.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Choices[0].Message.Content != "retry success" {
		t.Fatalf("unexpected response %q", resp.Choices[0].Message.Content)
	}
	if calls != 2 {
		t.Fatalf("expected 2 upstream calls, got %d", calls)
	}
	if len(slept) != 1 || slept[0] != time.Second {
		t.Fatalf("expected retry-after sleep of 1s, got %v", slept)
	}
}
