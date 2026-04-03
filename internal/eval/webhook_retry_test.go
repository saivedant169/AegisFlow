package eval

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestEvaluateRetriesOnceOn5xx(t *testing.T) {
	var attempts atomic.Int32
	received := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	we := NewWebhookEvaluator(server.URL, 1.0, 200*time.Millisecond, true)
	we.retryDelay = 5 * time.Millisecond
	we.Evaluate(WebhookRequest{Prompt: "hello", Response: "world"})

	timeout := time.After(500 * time.Millisecond)
	count := 0
	for count < 2 {
		select {
		case <-received:
			count++
		case <-timeout:
			t.Fatalf("expected 2 attempts, got %d", count)
		}
	}
}

func TestEvaluateRetriesOnceOnTimeout(t *testing.T) {
	var attempts atomic.Int32
	received := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		if attempts.Add(1) == 1 {
			time.Sleep(30 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	we := NewWebhookEvaluator(server.URL, 1.0, 10*time.Millisecond, true)
	we.retryDelay = 5 * time.Millisecond
	we.Evaluate(WebhookRequest{Prompt: "hello", Response: "world"})

	timeout := time.After(500 * time.Millisecond)
	count := 0
	for count < 2 {
		select {
		case <-received:
			count++
		case <-timeout:
			t.Fatalf("expected 2 attempts, got %d", count)
		}
	}
}
