package capability

import (
	"testing"
	"time"
)

func newTestIssuer() *Issuer {
	return NewIssuer([]byte("test-signing-key-32-bytes-long!!"))
}

func issueTestTicket(t *testing.T, iss *Issuer) *Ticket {
	t.Helper()
	ticket, err := iss.Issue(TicketRequest{
		Subject:    "agent-1",
		TaskID:     "task-1",
		SessionID:  "sess-1",
		Resource:   "repo/main",
		Verb:       "read",
		Protocol:   "git",
		Tool:       "git.list_branches",
		PolicyHash: "abc123",
		TTL:        5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	return ticket
}

func TestAdminAdapterActiveTickets_Empty(t *testing.T) {
	iss := newTestIssuer()
	adapter := NewAdminAdapter(iss)

	result := adapter.ActiveTickets()
	tickets, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{} for empty, got %T", result)
	}
	if len(tickets) != 0 {
		t.Fatalf("expected 0 tickets, got %d", len(tickets))
	}
}

func TestAdminAdapterActiveTickets_WithTicket(t *testing.T) {
	iss := newTestIssuer()
	adapter := NewAdminAdapter(iss)
	issueTestTicket(t, iss)

	result := adapter.ActiveTickets()
	tickets, ok := result.([]*Ticket)
	if !ok {
		t.Fatalf("expected []*Ticket, got %T", result)
	}
	if len(tickets) != 1 {
		t.Fatalf("expected 1 ticket, got %d", len(tickets))
	}
}

func TestAdminAdapterRevokeTicket(t *testing.T) {
	iss := newTestIssuer()
	adapter := NewAdminAdapter(iss)
	ticket := issueTestTicket(t, iss)

	if err := adapter.RevokeTicket(ticket.ID); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if !iss.IsRevoked(ticket.ID) {
		t.Fatal("ticket should be revoked")
	}
}

func TestAdminAdapterRevokeTicketEmptyID(t *testing.T) {
	iss := newTestIssuer()
	adapter := NewAdminAdapter(iss)

	if err := adapter.RevokeTicket(""); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestAdminAdapterVerifyTicket(t *testing.T) {
	iss := newTestIssuer()
	adapter := NewAdminAdapter(iss)
	ticket := issueTestTicket(t, iss)

	result, err := adapter.VerifyTicket(ticket.ID)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["valid"] != true {
		t.Fatalf("expected valid=true, got %v", m["valid"])
	}
}

func TestAdminAdapterVerifyTicketRevoked(t *testing.T) {
	iss := newTestIssuer()
	adapter := NewAdminAdapter(iss)
	ticket := issueTestTicket(t, iss)
	iss.Revoke(ticket.ID)

	// Revoked tickets are removed from the store, so VerifyTicket returns
	// "not found" — the ticket is no longer retrievable.
	_, err := adapter.VerifyTicket(ticket.ID)
	if err == nil {
		t.Fatal("expected error for revoked (removed) ticket")
	}
}

func TestAdminAdapterVerifyTicketNotFound(t *testing.T) {
	iss := newTestIssuer()
	adapter := NewAdminAdapter(iss)

	_, err := adapter.VerifyTicket("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent ticket")
	}
}

func TestAdminAdapterVerifyTicketEmptyID(t *testing.T) {
	iss := newTestIssuer()
	adapter := NewAdminAdapter(iss)

	_, err := adapter.VerifyTicket("")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}
