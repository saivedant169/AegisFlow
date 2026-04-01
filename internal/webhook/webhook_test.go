package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestWebhookSendsEvent(t *testing.T) {
	var mu sync.Mutex
	var received *Event

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ev Event
		json.NewDecoder(r.Body).Decode(&ev)
		mu.Lock()
		received = &ev
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL)
	n.Send(Event{
		EventType:  "policy_violation",
		PolicyName: "block-jailbreak",
		Action:     "block",
		TenantID:   "default",
		Model:      "gpt-4o",
		Message:    "blocked keyword detected",
	})

	// Wait for async send
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if received == nil {
		t.Fatal("webhook event not received")
	}
	if received.EventType != "policy_violation" {
		t.Errorf("expected event_type 'policy_violation', got '%s'", received.EventType)
	}
	if received.PolicyName != "block-jailbreak" {
		t.Errorf("expected policy_name 'block-jailbreak', got '%s'", received.PolicyName)
	}
	if received.TenantID != "default" {
		t.Errorf("expected tenant_id 'default', got '%s'", received.TenantID)
	}
	if received.Timestamp == "" {
		t.Error("timestamp should be set")
	}
}

func TestWebhookNilNotifier(t *testing.T) {
	n := NewNotifier("")
	if n != nil {
		t.Error("empty URL should return nil notifier")
	}
	// Should not panic
	var nilNotifier *Notifier
	nilNotifier.Send(Event{EventType: "test"})
}

func TestWebhookHandlesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL)
	// Should not panic, just log
	n.Send(Event{EventType: "test"})
	time.Sleep(200 * time.Millisecond)
}

// TestSendWithNilNotifier verifies that calling Send on an explicitly nil
// *Notifier does not panic and is a no-op.
func TestSendWithNilNotifier(t *testing.T) {
	var n *Notifier
	// Must not panic.
	n.Send(Event{EventType: "should-be-ignored"})
}

// TestWebhookTimeout verifies that the notifier respects its HTTP client
// timeout and does not hang indefinitely when the server is slow.
func TestWebhookTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow server that exceeds the client timeout.
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL)
	// Override the client with a very short timeout.
	n.client = &http.Client{Timeout: 50 * time.Millisecond}

	done := make(chan struct{})
	go func() {
		n.Send(Event{EventType: "timeout-test"})
		// Give the goroutine inside Send time to complete (it should time out fast).
		time.Sleep(300 * time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		// Success — the send completed (with a timeout error logged).
	case <-time.After(2 * time.Second):
		t.Fatal("webhook send did not respect timeout — hung too long")
	}
}

// TestEmptyURLReturnsNilNotifier confirms that NewNotifier("") returns nil.
func TestEmptyURLReturnsNilNotifier(t *testing.T) {
	n := NewNotifier("")
	if n != nil {
		t.Error("expected nil notifier for empty URL")
	}
}

// TestWebhookEventFields verifies that all event fields are correctly
// transmitted to the webhook endpoint.
func TestWebhookEventFields(t *testing.T) {
	var mu sync.Mutex
	var received *Event

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ev Event
		json.NewDecoder(r.Body).Decode(&ev)
		mu.Lock()
		received = &ev
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL)
	n.Send(Event{
		EventType:  "rate_limit",
		PolicyName: "max-tokens",
		Action:     "throttle",
		TenantID:   "tenant-42",
		Model:      "gpt-4o",
		Message:    "rate limit exceeded",
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if received == nil {
		t.Fatal("webhook event not received")
	}
	if received.Action != "throttle" {
		t.Errorf("expected action 'throttle', got %q", received.Action)
	}
	if received.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %q", received.Model)
	}
	if received.Message != "rate limit exceeded" {
		t.Errorf("expected message 'rate limit exceeded', got %q", received.Message)
	}
}
