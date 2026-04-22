package capability

import (
	"testing"
	"time"
)

func testKey() []byte {
	return []byte("test-signing-key-32bytes-long!!")
}

func validRequest() TicketRequest {
	return TicketRequest{
		Subject:     "agent-007",
		TaskID:      "task-123",
		SessionID:   "session-abc",
		EnvelopeID:  "env-456",
		Resource:    "repos/acme/widget",
		Verb:        "write",
		Protocol:    "http",
		Tool:        "github",
		PolicyHash:  "abc123policydef",
		EvidenceRef: "evidence-xyz",
		TTL:         5 * time.Minute,
	}
}

func TestIssueTicket(t *testing.T) {
	iss := NewIssuer(testKey())
	ticket, err := iss.Issue(validRequest())
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}

	if ticket.ID == "" {
		t.Error("expected non-empty ticket ID")
	}
	if ticket.Subject != "agent-007" {
		t.Errorf("Subject = %q, want %q", ticket.Subject, "agent-007")
	}
	if ticket.TaskID != "task-123" {
		t.Errorf("TaskID = %q, want %q", ticket.TaskID, "task-123")
	}
	if ticket.SessionID != "session-abc" {
		t.Errorf("SessionID = %q, want %q", ticket.SessionID, "session-abc")
	}
	if ticket.EnvelopeID != "env-456" {
		t.Errorf("EnvelopeID = %q, want %q", ticket.EnvelopeID, "env-456")
	}
	if ticket.Resource != "repos/acme/widget" {
		t.Errorf("Resource = %q, want %q", ticket.Resource, "repos/acme/widget")
	}
	if ticket.Verb != "write" {
		t.Errorf("Verb = %q, want %q", ticket.Verb, "write")
	}
	if ticket.Protocol != "http" {
		t.Errorf("Protocol = %q, want %q", ticket.Protocol, "http")
	}
	if ticket.Tool != "github" {
		t.Errorf("Tool = %q, want %q", ticket.Tool, "github")
	}
	if ticket.PolicyHash != "abc123policydef" {
		t.Errorf("PolicyHash = %q, want %q", ticket.PolicyHash, "abc123policydef")
	}
	if ticket.EvidenceRef != "evidence-xyz" {
		t.Errorf("EvidenceRef = %q, want %q", ticket.EvidenceRef, "evidence-xyz")
	}
	if ticket.Nonce == "" {
		t.Error("expected non-empty nonce")
	}
	if ticket.Signature == "" {
		t.Error("expected non-empty signature")
	}
	if ticket.IssuedAt.IsZero() {
		t.Error("expected non-zero IssuedAt")
	}
	if ticket.ExpiresAt.IsZero() {
		t.Error("expected non-zero ExpiresAt")
	}
	if !ticket.ExpiresAt.After(ticket.IssuedAt) {
		t.Error("ExpiresAt should be after IssuedAt")
	}
}

func TestVerifyValidTicket(t *testing.T) {
	iss := NewIssuer(testKey())
	ticket, err := iss.Issue(validRequest())
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}

	if err := iss.Verify(ticket); err != nil {
		t.Errorf("Verify() should succeed for valid ticket, got: %v", err)
	}
}

func TestVerifyExpiredTicket(t *testing.T) {
	iss := NewIssuer(testKey())
	req := validRequest()
	req.TTL = 1 * time.Millisecond
	ticket, err := iss.Issue(req)
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}

	// Wait for expiry.
	time.Sleep(5 * time.Millisecond)

	if err := iss.Verify(ticket); err == nil {
		t.Error("Verify() should return error for expired ticket")
	}
}

func TestVerifyTamperedTicket(t *testing.T) {
	iss := NewIssuer(testKey())
	ticket, err := iss.Issue(validRequest())
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}

	// Tamper with a field.
	ticket.Resource = "repos/evil/backdoor"

	if err := iss.Verify(ticket); err == nil {
		t.Error("Verify() should return error for tampered ticket")
	}
}

func TestVerifyRevokedTicket(t *testing.T) {
	iss := NewIssuer(testKey())
	ticket, err := iss.Issue(validRequest())
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}

	iss.Revoke(ticket.ID)

	if err := iss.Verify(ticket); err == nil {
		t.Error("Verify() should return error for revoked ticket")
	}

	if !iss.IsRevoked(ticket.ID) {
		t.Error("IsRevoked() should return true after revocation")
	}
}

func TestNonceUniqueness(t *testing.T) {
	iss := NewIssuer(testKey())
	t1, err := iss.Issue(validRequest())
	if err != nil {
		t.Fatalf("Issue() first error: %v", err)
	}
	t2, err := iss.Issue(validRequest())
	if err != nil {
		t.Fatalf("Issue() second error: %v", err)
	}

	if t1.Nonce == t2.Nonce {
		t.Error("two tickets should have different nonces")
	}
}

func TestTicketBoundToExactAction(t *testing.T) {
	iss := NewIssuer(testKey())
	req := validRequest()
	req.EnvelopeID = "specific-envelope-789"
	req.Resource = "exact/resource/path"
	req.Verb = "delete"

	ticket, err := iss.Issue(req)
	if err != nil {
		t.Fatalf("Issue() error: %v", err)
	}

	if ticket.EnvelopeID != "specific-envelope-789" {
		t.Errorf("EnvelopeID = %q, want %q", ticket.EnvelopeID, "specific-envelope-789")
	}
	if ticket.Resource != "exact/resource/path" {
		t.Errorf("Resource = %q, want %q", ticket.Resource, "exact/resource/path")
	}
	if ticket.Verb != "delete" {
		t.Errorf("Verb = %q, want %q", ticket.Verb, "delete")
	}

	// Verify the ticket is valid with these exact bindings.
	if err := iss.Verify(ticket); err != nil {
		t.Errorf("Verify() should succeed for correctly bound ticket, got: %v", err)
	}
}
