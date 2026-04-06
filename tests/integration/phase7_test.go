package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/credential"
)

// ---------- Phase 7: Credential Broker E2E Tests ----------

func TestCredentialIssuanceE2E(t *testing.T) {
	reg := credential.NewRegistry()
	broker := credential.NewStaticBroker("static", "tok-secret-123", 5*time.Minute)
	reg.Register("static", broker)

	req := credential.CredentialRequest{
		TaskID:    "task-issue-1",
		SessionID: "sess-1",
		TenantID:  "tenant-1",
		Tool:      "github.list_repos",
		Target:    "org/repo",
	}

	cred, prov, err := reg.Issue(context.Background(), "static", req, "env-001")
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	// Credential must have all required fields.
	if cred.ID == "" {
		t.Error("expected non-empty credential ID")
	}
	if cred.Token == "" {
		t.Error("expected non-empty token")
	}
	if cred.ExpiresAt.IsZero() {
		t.Error("expected non-zero expiry")
	}
	if cred.TaskID != "task-issue-1" {
		t.Errorf("expected TaskID 'task-issue-1', got %q", cred.TaskID)
	}
	if cred.Type != "static" {
		t.Errorf("expected Type 'static', got %q", cred.Type)
	}
	if cred.IssuedAt.IsZero() {
		t.Error("expected non-zero IssuedAt")
	}
	if time.Until(cred.ExpiresAt) <= 0 {
		t.Error("expected credential to not be expired immediately after issuance")
	}

	// Provenance must exist and match.
	if prov == nil {
		t.Fatal("expected non-nil provenance")
	}
	if prov.CredentialID != cred.ID {
		t.Errorf("provenance credential ID mismatch: %q vs %q", prov.CredentialID, cred.ID)
	}
	if prov.EnvelopeID != "env-001" {
		t.Errorf("expected envelope ID 'env-001', got %q", prov.EnvelopeID)
	}

	// Credential should appear in active list.
	active := reg.ActiveCredentials()
	if len(active) != 1 {
		t.Fatalf("expected 1 active credential, got %d", len(active))
	}
	if active[0].ID != cred.ID {
		t.Errorf("active credential ID mismatch")
	}
}

func TestCredentialRevocationE2E(t *testing.T) {
	reg := credential.NewRegistry()
	broker := credential.NewStaticBroker("static", "tok-revoke-123", 5*time.Minute)
	reg.Register("static", broker)

	req := credential.CredentialRequest{
		TaskID:    "task-revoke-1",
		SessionID: "sess-2",
		TenantID:  "tenant-1",
		Tool:      "shell.ls",
		Target:    "/tmp",
	}

	cred, _, err := reg.Issue(context.Background(), "static", req, "env-002")
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	// Verify it's active before revocation.
	active := reg.ActiveCredentials()
	if len(active) != 1 {
		t.Fatalf("expected 1 active credential before revoke, got %d", len(active))
	}

	// Revoke.
	if err := reg.Revoke(context.Background(), cred.ID); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	// Verify removed from active list.
	active = reg.ActiveCredentials()
	if len(active) != 0 {
		t.Fatalf("expected 0 active credentials after revoke, got %d", len(active))
	}

	// Revoking again should return an error.
	err = reg.Revoke(context.Background(), cred.ID)
	if err == nil {
		t.Error("expected error revoking already-revoked credential")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %q", err.Error())
	}
}

func TestCredentialExpiryCleanupE2E(t *testing.T) {
	reg := credential.NewRegistry()
	broker := credential.NewStaticBroker("static", "tok-expire-123", 5*time.Minute)
	reg.Register("static", broker)

	// Issue with very short TTL.
	req := credential.CredentialRequest{
		TaskID:    "task-expire-1",
		SessionID: "sess-3",
		TenantID:  "tenant-1",
		Tool:      "sql.select",
		Target:    "db-production",
		TTL:       1 * time.Millisecond,
	}

	_, _, err := reg.Issue(context.Background(), "static", req, "env-003")
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	// Wait for expiry.
	time.Sleep(5 * time.Millisecond)

	// Before cleanup, the credential is still tracked (but expired).
	// ActiveCredentials filters out expired ones.
	active := reg.ActiveCredentials()
	if len(active) != 0 {
		t.Fatalf("expected 0 active (non-expired) credentials, got %d", len(active))
	}

	// Cleanup should remove the expired entry from internal tracking.
	reg.CleanupExpired()

	// Verify it's gone.
	active = reg.ActiveCredentials()
	if len(active) != 0 {
		t.Fatalf("expected 0 active credentials after cleanup, got %d", len(active))
	}
}

func TestCredentialProvenanceE2E(t *testing.T) {
	reg := credential.NewRegistry()
	broker := credential.NewStaticBroker("static", "super-secret-token", 5*time.Minute)
	reg.Register("static", broker)

	req := credential.CredentialRequest{
		TaskID:    "task-prov-1",
		SessionID: "sess-4",
		TenantID:  "tenant-1",
		Tool:      "github.create_pr",
		Target:    "myorg/myrepo",
	}

	cred, prov, err := reg.Issue(context.Background(), "static", req, "env-004")
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	// Provenance should have correct metadata.
	if prov.CredentialID != cred.ID {
		t.Errorf("provenance credential ID mismatch")
	}
	if prov.BrokerName != "static" {
		t.Errorf("expected broker name 'static', got %q", prov.BrokerName)
	}
	if prov.EnvelopeID != "env-004" {
		t.Errorf("expected envelope ID 'env-004', got %q", prov.EnvelopeID)
	}
	if prov.TaskID != "task-prov-1" {
		t.Errorf("expected task ID 'task-prov-1', got %q", prov.TaskID)
	}
	if prov.Type != "static" {
		t.Errorf("expected type 'static', got %q", prov.Type)
	}
	if prov.Scope != "myorg/myrepo" {
		t.Errorf("expected scope 'myorg/myrepo', got %q", prov.Scope)
	}

	// Provenance must NOT leak the secret token.
	provJSON, err := json.Marshal(prov)
	if err != nil {
		t.Fatalf("marshal provenance: %v", err)
	}
	provStr := string(provJSON)
	if strings.Contains(provStr, "super-secret-token") {
		t.Error("provenance JSON contains the secret token -- secret leakage detected")
	}
	if strings.Contains(provStr, cred.Token) {
		t.Error("provenance JSON contains the credential token value -- secret leakage detected")
	}
}

// mockGitHubBroker is a test-only broker that mimics a GitHub App credential broker.
type mockGitHubBroker struct {
	revoked map[string]bool
}

func (b *mockGitHubBroker) Name() string { return "github_app" }

func (b *mockGitHubBroker) Issue(_ context.Context, req credential.CredentialRequest) (*credential.Credential, error) {
	now := time.Now().UTC()
	ttl := 10 * time.Minute
	if req.TTL > 0 {
		ttl = req.TTL
	}
	return &credential.Credential{
		ID:        "gh-cred-" + req.TaskID,
		Type:      "github_app",
		Token:     "ghs_mock_installation_token",
		ExpiresAt: now.Add(ttl),
		Scope:     req.Target,
		TaskID:    req.TaskID,
		IssuedAt:  now,
	}, nil
}

func (b *mockGitHubBroker) Revoke(_ context.Context, credID string) error {
	if b.revoked == nil {
		b.revoked = make(map[string]bool)
	}
	b.revoked[credID] = true
	return nil
}

func TestMultipleBrokersE2E(t *testing.T) {
	reg := credential.NewRegistry()

	staticBroker := credential.NewStaticBroker("static", "static-tok", 5*time.Minute)
	ghBroker := &mockGitHubBroker{}

	reg.Register("static", staticBroker)
	reg.Register("github_app", ghBroker)

	// Issue from static broker.
	staticReq := credential.CredentialRequest{
		TaskID:    "task-multi-static",
		SessionID: "sess-5",
		TenantID:  "tenant-1",
		Tool:      "shell.ls",
		Target:    "/home",
	}
	staticCred, staticProv, err := reg.Issue(context.Background(), "static", staticReq, "env-005a")
	if err != nil {
		t.Fatalf("static issue failed: %v", err)
	}
	if staticCred.Type != "static" {
		t.Errorf("expected static type, got %q", staticCred.Type)
	}
	if staticProv.BrokerName != "static" {
		t.Errorf("expected broker name 'static', got %q", staticProv.BrokerName)
	}

	// Issue from github_app broker.
	ghReq := credential.CredentialRequest{
		TaskID:    "task-multi-gh",
		SessionID: "sess-5",
		TenantID:  "tenant-1",
		Tool:      "github.create_pr",
		Target:    "myorg/myrepo",
	}
	ghCred, ghProv, err := reg.Issue(context.Background(), "github_app", ghReq, "env-005b")
	if err != nil {
		t.Fatalf("github_app issue failed: %v", err)
	}
	if ghCred.Type != "github_app" {
		t.Errorf("expected github_app type, got %q", ghCred.Type)
	}
	if ghProv.BrokerName != "github_app" {
		t.Errorf("expected broker name 'github_app', got %q", ghProv.BrokerName)
	}

	// Both should be active.
	active := reg.ActiveCredentials()
	if len(active) != 2 {
		t.Fatalf("expected 2 active credentials, got %d", len(active))
	}

	// Verify they are different types.
	types := map[string]bool{}
	for _, c := range active {
		types[c.Type] = true
	}
	if !types["static"] || !types["github_app"] {
		t.Errorf("expected both static and github_app types in active credentials, got %v", types)
	}

	// Issuing from unregistered broker should fail.
	_, _, err = reg.Issue(context.Background(), "vault", ghReq, "env-005c")
	if err == nil {
		t.Error("expected error for unregistered broker 'vault'")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected 'not registered' error, got %q", err.Error())
	}
}
