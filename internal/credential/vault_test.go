package credential

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVaultBrokerIssue(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "test-token" {
			t.Errorf("expected X-Vault-Token=test-token, got %q", r.Header.Get("X-Vault-Token"))
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/creds/my-role") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"request_id":     "req-123",
			"lease_id":       "database/creds/my-role/abc123",
			"renewable":      true,
			"lease_duration": 3600,
			"data": map[string]string{
				"username": "v-token-my-role-abc",
				"password": "s3cret-pw",
			},
		})
	}))
	defer ts.Close()

	broker := NewVaultBroker("vault-db", ts.URL, "test-token", "database", 30*time.Minute, ts.Client())

	cred, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-1",
		Target: "my-role",
	})
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}
	if cred.Type != "vault" {
		t.Errorf("expected type vault, got %s", cred.Type)
	}
	if cred.Token != "v-token-my-role-abc:s3cret-pw" {
		t.Errorf("unexpected token: %s", cred.Token)
	}
	if cred.Scope != "my-role" {
		t.Errorf("unexpected scope: %s", cred.Scope)
	}
	if cred.TaskID != "task-1" {
		t.Errorf("unexpected task ID: %s", cred.TaskID)
	}
	if cred.ExpiresAt.Before(time.Now()) {
		t.Error("credential already expired")
	}
}

func TestVaultBrokerRevoke(t *testing.T) {
	var revokedLeaseID string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/creds/my-role") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"lease_id":       "database/creds/my-role/abc123",
				"lease_duration": 3600,
				"data": map[string]string{
					"username": "u",
					"password": "p",
				},
			})
			return
		}
		if r.URL.Path == "/v1/sys/leases/revoke" && r.Method == http.MethodPut {
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			revokedLeaseID = body["lease_id"]
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer ts.Close()

	broker := NewVaultBroker("vault-db", ts.URL, "test-token", "database", 30*time.Minute, ts.Client())

	cred, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-1",
		Target: "my-role",
	})
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}

	err = broker.Revoke(context.Background(), cred.ID)
	if err != nil {
		t.Fatalf("Revoke() error: %v", err)
	}
	if revokedLeaseID != "database/creds/my-role/abc123" {
		t.Errorf("unexpected revoked lease ID: %s", revokedLeaseID)
	}
}

func TestVaultBrokerAuthError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errors":["permission denied"]}`))
	}))
	defer ts.Close()

	broker := NewVaultBroker("vault-db", ts.URL, "bad-token", "database", 30*time.Minute, ts.Client())

	_, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-1",
		Target: "my-role",
	})
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403 Forbidden") {
		t.Errorf("expected 403 in error, got: %v", err)
	}
}

func TestVaultBrokerNotFound(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errors":["no handler for route"]}`))
	}))
	defer ts.Close()

	broker := NewVaultBroker("vault-db", ts.URL, "test-token", "database", 30*time.Minute, ts.Client())

	_, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-1",
		Target: "nonexistent-role",
	})
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error, got: %v", err)
	}
}

func TestVaultBrokerTTL(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"lease_id":       "database/creds/my-role/ttl-test",
			"lease_duration": 300, // 5 minutes from Vault
			"data": map[string]string{
				"username": "u",
				"password": "p",
			},
		})
	}))
	defer ts.Close()

	broker := NewVaultBroker("vault-db", ts.URL, "test-token", "database", 1*time.Hour, ts.Client())

	// Vault returns 300s lease; request TTL is shorter at 2 minutes — should use 2m.
	cred, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-1",
		Target: "my-role",
		TTL:    2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}

	remaining := time.Until(cred.ExpiresAt)
	if remaining > 2*time.Minute+time.Second || remaining < 1*time.Minute+50*time.Second {
		t.Errorf("expected ~2m TTL, got %s", remaining)
	}

	// No request TTL — should use Vault's 300s lease duration.
	cred2, err := broker.Issue(context.Background(), CredentialRequest{
		TaskID: "task-2",
		Target: "my-role",
	})
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}

	remaining2 := time.Until(cred2.ExpiresAt)
	if remaining2 > 5*time.Minute+time.Second || remaining2 < 4*time.Minute+50*time.Second {
		t.Errorf("expected ~5m TTL from Vault lease, got %s", remaining2)
	}
}
