package credential

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStaticBrokerIssue(t *testing.T) {
	broker := NewStaticBroker("test-static", "my-secret-token", 30*time.Minute)

	cred, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID:     "task-1",
		SessionID:  "sess-1",
		TenantID:   "tenant-1",
		Tool:       "github.list_repos",
		Target:     "repos",
		Capability: "read",
		TTL:        10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cred.Token != "my-secret-token" {
		t.Errorf("expected token 'my-secret-token', got %q", cred.Token)
	}
	if cred.Type != "static" {
		t.Errorf("expected type 'static', got %q", cred.Type)
	}
	if cred.TaskID != "task-1" {
		t.Errorf("expected task_id 'task-1', got %q", cred.TaskID)
	}
	if cred.ID == "" {
		t.Error("expected non-empty credential ID")
	}
	if cred.Scope != "repos" {
		t.Errorf("expected scope 'repos', got %q", cred.Scope)
	}
	// Should expire in ~10 minutes (the request TTL, not the broker default).
	if time.Until(cred.ExpiresAt) > 11*time.Minute || time.Until(cred.ExpiresAt) < 9*time.Minute {
		t.Errorf("expected expiry around 10 minutes, got %s", time.Until(cred.ExpiresAt))
	}
}

func TestStaticBrokerRevoke(t *testing.T) {
	broker := NewStaticBroker("test-static", "tok", 1*time.Hour)

	cred, err := broker.Issue(context.Background(), CredentialRequest{TaskID: "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := broker.Revoke(context.Background(), cred.ID); err != nil {
		t.Fatalf("unexpected revoke error: %v", err)
	}

	// Revoking again should also succeed (idempotent).
	if err := broker.Revoke(context.Background(), cred.ID); err != nil {
		t.Fatalf("unexpected re-revoke error: %v", err)
	}
}

func TestRegistryIssue(t *testing.T) {
	reg := NewRegistry()
	broker := NewStaticBroker("static", "tok-123", 1*time.Hour)
	reg.Register("static", broker)

	cred, err := reg.Issue(context.Background(), "static", CredentialRequest{
		TaskID: "task-reg-1",
		Target: "api",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Token != "tok-123" {
		t.Errorf("expected token 'tok-123', got %q", cred.Token)
	}
	if cred.TaskID != "task-reg-1" {
		t.Errorf("expected task_id 'task-reg-1', got %q", cred.TaskID)
	}

	// Issuing from unknown broker should fail.
	_, err = reg.Issue(context.Background(), "unknown", CredentialRequest{})
	if err == nil {
		t.Error("expected error for unknown broker")
	}
}

func TestRegistryRevoke(t *testing.T) {
	reg := NewRegistry()
	broker := NewStaticBroker("static", "tok", 1*time.Hour)
	reg.Register("static", broker)

	cred, err := reg.Issue(context.Background(), "static", CredentialRequest{TaskID: "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := reg.Revoke(context.Background(), cred.ID); err != nil {
		t.Fatalf("unexpected revoke error: %v", err)
	}

	// Revoking again should fail (already removed).
	if err := reg.Revoke(context.Background(), cred.ID); err == nil {
		t.Error("expected error revoking already-revoked credential")
	}
}

func TestRegistryActiveCredentials(t *testing.T) {
	reg := NewRegistry()
	broker := NewStaticBroker("static", "tok", 1*time.Hour)
	reg.Register("static", broker)

	// Issue two credentials.
	_, err := reg.Issue(context.Background(), "static", CredentialRequest{TaskID: "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = reg.Issue(context.Background(), "static", CredentialRequest{TaskID: "t2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	active := reg.ActiveCredentials()
	if len(active) != 2 {
		t.Errorf("expected 2 active credentials, got %d", len(active))
	}
}

func TestRegistryCleanupExpired(t *testing.T) {
	reg := NewRegistry()
	broker := NewStaticBroker("static", "tok", 1*time.Millisecond) // very short TTL
	reg.Register("static", broker)

	_, err := reg.Issue(context.Background(), "static", CredentialRequest{TaskID: "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for expiry.
	time.Sleep(5 * time.Millisecond)

	reg.CleanupExpired()
	active := reg.ActiveCredentials()
	if len(active) != 0 {
		t.Errorf("expected 0 active credentials after cleanup, got %d", len(active))
	}
}

func TestGitHubAppBrokerIssue(t *testing.T) {
	// Set up a mock GitHub API server.
	expiresAt := time.Now().UTC().Add(1 * time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request path and auth header.
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/app/installations/12345/access_tokens" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer fake-jwt" {
			t.Errorf("unexpected auth header: %s", auth)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      "ghs_test_installation_token",
			"expires_at": expiresAt.Format(time.RFC3339),
		})
	}))
	defer server.Close()

	broker := NewGitHubAppBroker(
		"github",
		99,
		"/fake/key.pem",
		12345,
		1*time.Hour,
		WithGitHubBaseURL(server.URL),
		WithJWTFunc(func() (string, error) {
			return "fake-jwt", nil
		}),
	)

	cred, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID:     "task-gh-1",
		Target:     "repos",
		Capability: "read",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cred.Token != "ghs_test_installation_token" {
		t.Errorf("expected token 'ghs_test_installation_token', got %q", cred.Token)
	}
	if cred.Type != "github_app" {
		t.Errorf("expected type 'github_app', got %q", cred.Type)
	}
	if cred.TaskID != "task-gh-1" {
		t.Errorf("expected task_id 'task-gh-1', got %q", cred.TaskID)
	}
}

func TestCredentialExpiry(t *testing.T) {
	broker := NewStaticBroker("test", "tok", 1*time.Millisecond)

	cred, err := broker.Issue(context.Background(), CredentialRequest{TaskID: "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not be expired immediately.
	if cred.Expired() {
		t.Error("credential should not be expired immediately after issuance")
	}

	// Wait for expiry.
	time.Sleep(5 * time.Millisecond)

	if !cred.Expired() {
		t.Error("credential should be expired after waiting")
	}
}

func TestIssueSetsTaskID(t *testing.T) {
	broker := NewStaticBroker("test", "tok", 1*time.Hour)

	cred, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID:    "my-unique-task-id",
		SessionID: "sess-42",
		TenantID:  "acme",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cred.TaskID != "my-unique-task-id" {
		t.Errorf("expected task_id 'my-unique-task-id', got %q", cred.TaskID)
	}
}
