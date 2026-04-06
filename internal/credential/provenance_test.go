package credential

import (
	"encoding/json"
	"testing"
	"time"
)

func TestToProvenance(t *testing.T) {
	now := time.Now().UTC()
	cred := &Credential{
		ID:        "cred-001",
		Type:      "github_app",
		Token:     "super-secret-token",
		ExpiresAt: now.Add(1 * time.Hour),
		Scope:     "repos:read",
		TaskID:    "task-42",
		IssuedAt:  now,
	}

	prov := ToProvenance(cred, "github", "env-abc")
	if prov == nil {
		t.Fatal("expected non-nil provenance")
	}
	if prov.CredentialID != "cred-001" {
		t.Errorf("expected credential_id 'cred-001', got %q", prov.CredentialID)
	}
	if prov.BrokerName != "github" {
		t.Errorf("expected broker_name 'github', got %q", prov.BrokerName)
	}
	if prov.Type != "github_app" {
		t.Errorf("expected type 'github_app', got %q", prov.Type)
	}
	if prov.Scope != "repos:read" {
		t.Errorf("expected scope 'repos:read', got %q", prov.Scope)
	}
	if prov.TaskID != "task-42" {
		t.Errorf("expected task_id 'task-42', got %q", prov.TaskID)
	}
	if prov.EnvelopeID != "env-abc" {
		t.Errorf("expected envelope_id 'env-abc', got %q", prov.EnvelopeID)
	}
	if !prov.IssuedAt.Equal(now) {
		t.Errorf("expected issued_at %v, got %v", now, prov.IssuedAt)
	}
	if !prov.ExpiresAt.Equal(now.Add(1 * time.Hour)) {
		t.Errorf("expected expires_at %v, got %v", now.Add(1*time.Hour), prov.ExpiresAt)
	}
}

func TestToProvenanceNilCredential(t *testing.T) {
	prov := ToProvenance(nil, "github", "env-abc")
	if prov != nil {
		t.Error("expected nil provenance for nil credential")
	}
}

func TestProvenanceJSON(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	prov := &CredentialProvenance{
		CredentialID: "cred-json-1",
		BrokerName:   "vault",
		Type:         "vault",
		Scope:        "secret/data/myapp",
		IssuedAt:     now,
		ExpiresAt:    now.Add(30 * time.Minute),
		TaskID:       "task-json-1",
		EnvelopeID:   "env-json-1",
	}

	data, err := json.Marshal(prov)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Verify the secret token is NOT in the JSON output.
	if string(data) == "" {
		t.Fatal("expected non-empty JSON")
	}

	var decoded CredentialProvenance
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.CredentialID != prov.CredentialID {
		t.Errorf("credential_id mismatch: %q vs %q", decoded.CredentialID, prov.CredentialID)
	}
	if decoded.BrokerName != prov.BrokerName {
		t.Errorf("broker_name mismatch: %q vs %q", decoded.BrokerName, prov.BrokerName)
	}
	if decoded.Type != prov.Type {
		t.Errorf("type mismatch: %q vs %q", decoded.Type, prov.Type)
	}
	if decoded.Scope != prov.Scope {
		t.Errorf("scope mismatch: %q vs %q", decoded.Scope, prov.Scope)
	}
	if decoded.TaskID != prov.TaskID {
		t.Errorf("task_id mismatch: %q vs %q", decoded.TaskID, prov.TaskID)
	}
	if decoded.EnvelopeID != prov.EnvelopeID {
		t.Errorf("envelope_id mismatch: %q vs %q", decoded.EnvelopeID, prov.EnvelopeID)
	}
	if !decoded.IssuedAt.Equal(prov.IssuedAt) {
		t.Errorf("issued_at mismatch: %v vs %v", decoded.IssuedAt, prov.IssuedAt)
	}
	if !decoded.ExpiresAt.Equal(prov.ExpiresAt) {
		t.Errorf("expires_at mismatch: %v vs %v", decoded.ExpiresAt, prov.ExpiresAt)
	}
}

func TestProvenanceLinkedToEnvelope(t *testing.T) {
	now := time.Now().UTC()
	cred := &Credential{
		ID:        "cred-link-1",
		Type:      "static",
		Token:     "should-not-appear",
		ExpiresAt: now.Add(15 * time.Minute),
		Scope:     "api:write",
		TaskID:    "task-link-1",
		IssuedAt:  now,
	}

	envelopeID := "env-link-abc-123"
	prov := ToProvenance(cred, "static", envelopeID)

	// Verify the provenance links to the envelope.
	if prov.EnvelopeID != envelopeID {
		t.Errorf("expected envelope_id %q, got %q", envelopeID, prov.EnvelopeID)
	}

	// Verify the secret token is NOT in the provenance.
	data, err := json.Marshal(prov)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	jsonStr := string(data)
	if contains(jsonStr, "should-not-appear") {
		t.Error("provenance JSON must not contain the secret token")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
