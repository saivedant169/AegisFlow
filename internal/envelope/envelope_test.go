package envelope

import (
	"strings"
	"testing"
	"time"
)

func validActor() ActorInfo {
	return ActorInfo{
		Type:      "agent",
		ID:        "agent-001",
		SessionID: "sess-abc",
		TenantID:  "tenant-xyz",
	}
}

func TestNewEnvelope(t *testing.T) {
	env := NewEnvelope(validActor(), "deploy-service", ProtocolHTTP, "kubectl", "k8s://prod/deploy", CapDeploy)

	if env.ID == "" {
		t.Fatal("expected ID to be set")
	}
	if env.Timestamp.IsZero() {
		t.Fatal("expected Timestamp to be set")
	}
	if time.Since(env.Timestamp) > 5*time.Second {
		t.Fatal("expected Timestamp to be recent")
	}
	if env.PolicyDecision != DecisionPending {
		t.Fatalf("expected PolicyDecision to be pending, got %q", env.PolicyDecision)
	}
	if env.Parameters == nil {
		t.Fatal("expected Parameters map to be initialized")
	}
}

func TestHashDeterministic(t *testing.T) {
	env := &ActionEnvelope{
		ID:                  "fixed-id",
		Timestamp:           time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC),
		Actor:               validActor(),
		Task:                "read-logs",
		Protocol:            ProtocolHTTP,
		Tool:                "curl",
		Target:              "https://logs.example.com",
		Parameters:          map[string]any{"limit": 100},
		RequestedCapability: CapRead,
		PolicyDecision:      DecisionAllow,
	}

	h1 := env.Hash()
	h2 := env.Hash()

	if h1 != h2 {
		t.Fatalf("hash not deterministic: %q != %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Fatalf("expected 64-char hex SHA-256 hash, got length %d", len(h1))
	}
}

func TestHashChangesWhenContentChanges(t *testing.T) {
	env := &ActionEnvelope{
		ID:                  "fixed-id",
		Timestamp:           time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC),
		Actor:               validActor(),
		Task:                "read-logs",
		Protocol:            ProtocolHTTP,
		Tool:                "curl",
		Target:              "https://logs.example.com",
		Parameters:          map[string]any{"limit": 100},
		RequestedCapability: CapRead,
		PolicyDecision:      DecisionAllow,
	}

	h1 := env.Hash()

	env.Target = "https://other.example.com"
	h2 := env.Hash()

	if h1 == h2 {
		t.Fatal("expected hash to change when target changes")
	}
}

func TestIsDestructive(t *testing.T) {
	tests := []struct {
		cap        Capability
		destructive bool
	}{
		{CapRead, false},
		{CapWrite, true},
		{CapDelete, true},
		{CapDeploy, true},
		{CapApprove, false},
		{CapExecute, false},
	}

	for _, tc := range tests {
		env := &ActionEnvelope{RequestedCapability: tc.cap}
		got := env.IsDestructive()
		if got != tc.destructive {
			t.Errorf("IsDestructive() for %q: got %v, want %v", tc.cap, got, tc.destructive)
		}
	}
}

func TestValidateMissingRequiredFields(t *testing.T) {
	env := &ActionEnvelope{}
	err := env.Validate()
	if err == nil {
		t.Fatal("expected validation error for empty envelope")
	}

	msg := err.Error()
	required := []string{
		"id is required",
		"timestamp is required",
		"actor.type is required",
		"actor.id is required",
		"task is required",
		"protocol is required",
		"tool is required",
		"target is required",
		"requested_capability is required",
	}
	for _, r := range required {
		if !strings.Contains(msg, r) {
			t.Errorf("expected error to contain %q, got: %s", r, msg)
		}
	}
}

func TestValidatePassesForValidEnvelope(t *testing.T) {
	env := NewEnvelope(validActor(), "task-1", ProtocolMCP, "tool-a", "/target", CapRead)
	if err := env.Validate(); err != nil {
		t.Fatalf("expected no error for valid envelope, got: %v", err)
	}
}

func TestProtocolConstants(t *testing.T) {
	tests := []struct {
		p    Protocol
		want string
	}{
		{ProtocolMCP, "mcp"},
		{ProtocolHTTP, "http"},
		{ProtocolShell, "shell"},
		{ProtocolSQL, "sql"},
		{ProtocolGit, "git"},
	}
	for _, tc := range tests {
		if string(tc.p) != tc.want {
			t.Errorf("Protocol %v: got %q, want %q", tc.p, string(tc.p), tc.want)
		}
	}
}

func TestDecisionConstants(t *testing.T) {
	tests := []struct {
		d    Decision
		want string
	}{
		{DecisionPending, "pending"},
		{DecisionAllow, "allow"},
		{DecisionReview, "review"},
		{DecisionBlock, "block"},
	}
	for _, tc := range tests {
		if string(tc.d) != tc.want {
			t.Errorf("Decision %v: got %q, want %q", tc.d, string(tc.d), tc.want)
		}
	}
}
