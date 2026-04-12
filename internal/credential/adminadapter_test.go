package credential

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAdminAdapterActiveCredentials_Empty(t *testing.T) {
	reg := NewRegistry()
	adapter := NewAdminAdapter(reg)

	result := adapter.ActiveCredentials()
	creds, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result)
	}
	if len(creds) != 0 {
		t.Fatalf("expected 0 credentials, got %d", len(creds))
	}
}

func TestAdminAdapterActiveCredentials_Redacted(t *testing.T) {
	reg := NewRegistry()
	reg.Register("static", NewStaticBroker("static", "super-secret-token-12345", 10*time.Minute))

	adapter := NewAdminAdapter(reg)

	// Issue a credential so there's something active
	req := CredentialRequest{TaskID: "task-1", Target: "repo", Capability: "read", TTL: 10 * time.Minute}
	_, _, err := reg.Issue(nil, "static", req, "env-1")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}

	result := adapter.ActiveCredentials()

	// Marshal to JSON and back to inspect the redacted fields
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var creds []map[string]interface{}
	if err := json.Unmarshal(data, &creds); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(creds))
	}

	// Token should be redacted (first 8 chars + "...")
	hint := creds[0]["token_hint"].(string)
	if !strings.HasSuffix(hint, "...") {
		t.Errorf("expected redacted token hint, got %q", hint)
	}
	if strings.Contains(hint, "super-secret-token-12345") {
		t.Error("token hint should not contain full token")
	}
	if creds[0]["task_id"] != "task-1" {
		t.Errorf("expected task-1, got %v", creds[0]["task_id"])
	}
}

func TestAdminAdapterRevokeCredential(t *testing.T) {
	reg := NewRegistry()
	reg.Register("static", NewStaticBroker("static", "tok", 10*time.Minute))
	adapter := NewAdminAdapter(reg)

	req := CredentialRequest{TaskID: "t1", Target: "r", Capability: "read", TTL: 5 * time.Minute}
	cred, _, _ := reg.Issue(nil, "static", req, "env-1")

	if err := adapter.RevokeCredential(cred.ID); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}

	// Should be gone from active
	active := reg.ActiveCredentials()
	for _, c := range active {
		if c.ID == cred.ID {
			t.Fatal("credential should have been revoked")
		}
	}
}

func TestAdminAdapterRevokeCredentialEmptyID(t *testing.T) {
	reg := NewRegistry()
	adapter := NewAdminAdapter(reg)

	if err := adapter.RevokeCredential(""); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestAdminAdapterRevokeCredentialNotFound(t *testing.T) {
	reg := NewRegistry()
	adapter := NewAdminAdapter(reg)

	if err := adapter.RevokeCredential("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent ID")
	}
}

func TestAdminAdapterIssueCredential(t *testing.T) {
	reg := NewRegistry()
	reg.Register("static", NewStaticBroker("static", "tok", 10*time.Minute))
	adapter := NewAdminAdapter(reg)

	prov, err := adapter.IssueCredential("static", "task-1", "repo", "read", "env-1")
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	if prov == nil {
		t.Fatal("expected non-nil provenance")
	}
}

func TestAdminAdapterIssueCredentialUnknownBroker(t *testing.T) {
	reg := NewRegistry()
	adapter := NewAdminAdapter(reg)

	_, err := adapter.IssueCredential("nonexistent", "task-1", "repo", "read", "env-1")
	if err == nil {
		t.Fatal("expected error for unknown broker")
	}
}
