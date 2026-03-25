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
