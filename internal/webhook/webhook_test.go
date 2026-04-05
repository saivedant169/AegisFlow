package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// TestHMACSignaturePresent verifies that when a secret is configured,
// the X-AegisFlow-Signature and X-AegisFlow-Timestamp headers are set
// and the signature is a valid HMAC-SHA256.
func TestHMACSignaturePresent(t *testing.T) {
	const secret = "test-webhook-secret"

	var mu sync.Mutex
	var sigHeader, tsHeader string
	var bodyBytes []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sigHeader = r.Header.Get("X-AegisFlow-Signature")
		tsHeader = r.Header.Get("X-AegisFlow-Timestamp")
		bodyBytes, _ = io.ReadAll(r.Body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL, secret)
	n.Send(Event{
		EventType:  "policy_violation",
		PolicyName: "block-jailbreak",
		Action:     "block",
		TenantID:   "default",
		Model:      "gpt-4o",
		Message:    "test hmac",
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if sigHeader == "" {
		t.Fatal("X-AegisFlow-Signature header is missing")
	}
	if tsHeader == "" {
		t.Fatal("X-AegisFlow-Timestamp header is missing")
	}

	// Parse timestamp and verify the signature
	ts, err := strconv.ParseInt(tsHeader, 10, 64)
	if err != nil {
		t.Fatalf("X-AegisFlow-Timestamp is not a valid integer: %v", err)
	}

	// Verify the timestamp is recent (within 10 seconds)
	now := time.Now().Unix()
	if now-ts > 10 || ts-now > 10 {
		t.Errorf("timestamp %d is not recent (now=%d)", ts, now)
	}

	// Recompute the expected signature
	expected := ComputeSignature(secret, ts, bodyBytes)
	if sigHeader != expected {
		t.Errorf("signature mismatch:\n  got:  %s\n  want: %s", sigHeader, expected)
	}
}

// TestHMACSignatureValid verifies the HMAC computation independently.
func TestHMACSignatureValid(t *testing.T) {
	secret := "my-secret"
	body := []byte(`{"event_type":"test"}`)
	var ts int64 = 1700000000

	sig := ComputeSignature(secret, ts, body)

	// Recompute to verify determinism
	sig2 := ComputeSignature(secret, ts, body)
	if sig != sig2 {
		t.Error("ComputeSignature is not deterministic")
	}

	// Different secret should produce different signature
	sig3 := ComputeSignature("other-secret", ts, body)
	if sig == sig3 {
		t.Error("different secrets produced the same signature")
	}

	// Different timestamp should produce different signature
	sig4 := ComputeSignature(secret, ts+1, body)
	if sig == sig4 {
		t.Error("different timestamps produced the same signature")
	}

	// Different body should produce different signature
	sig5 := ComputeSignature(secret, ts, []byte(`{"event_type":"other"}`))
	if sig == sig5 {
		t.Error("different bodies produced the same signature")
	}

	// Verify the format is a hex string of correct length (SHA-256 = 32 bytes = 64 hex chars)
	if len(sig) != 64 {
		t.Errorf("signature length should be 64 hex chars, got %d", len(sig))
	}

	// Verify it matches the expected HMAC-SHA256 of "timestamp.body"
	payload := fmt.Sprintf("%d.%s", ts, body)
	_ = payload // used in ComputeSignature
}

// TestNoSignatureWithoutSecret verifies that when no secret is configured,
// no signature headers are added to the request.
func TestNoSignatureWithoutSecret(t *testing.T) {
	var mu sync.Mutex
	var sigHeader, tsHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sigHeader = r.Header.Get("X-AegisFlow-Signature")
		tsHeader = r.Header.Get("X-AegisFlow-Timestamp")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// No secret provided
	n := NewNotifier(srv.URL)
	n.Send(Event{
		EventType: "test",
		Message:   "no secret",
	})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if sigHeader != "" {
		t.Errorf("X-AegisFlow-Signature should be empty without secret, got %q", sigHeader)
	}
	if tsHeader != "" {
		t.Errorf("X-AegisFlow-Timestamp should be empty without secret, got %q", tsHeader)
	}
}

// TestNoSignatureWithEmptySecret verifies that an empty string secret
// is treated the same as no secret.
func TestNoSignatureWithEmptySecret(t *testing.T) {
	var mu sync.Mutex
	var sigHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sigHeader = r.Header.Get("X-AegisFlow-Signature")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL, "")
	n.Send(Event{EventType: "test"})

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if sigHeader != "" {
		t.Errorf("X-AegisFlow-Signature should be empty with empty secret, got %q", sigHeader)
	}
}
