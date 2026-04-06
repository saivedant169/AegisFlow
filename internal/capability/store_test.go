package capability

import (
	"testing"
	"time"
)

func TestStoreActiveTickets(t *testing.T) {
	s := NewStore()

	t1 := &Ticket{
		ID:        "ticket-1",
		Subject:   "agent-1",
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
	}
	t2 := &Ticket{
		ID:        "ticket-2",
		Subject:   "agent-2",
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
	}

	s.Add(t1)
	s.Add(t2)

	active := s.ActiveTickets()
	if len(active) != 2 {
		t.Errorf("ActiveTickets() count = %d, want 2", len(active))
	}

	got := s.Get("ticket-1")
	if got == nil {
		t.Fatal("Get(ticket-1) returned nil")
	}
	if got.Subject != "agent-1" {
		t.Errorf("Get(ticket-1).Subject = %q, want %q", got.Subject, "agent-1")
	}

	if s.ActiveCount() != 2 {
		t.Errorf("ActiveCount() = %d, want 2", s.ActiveCount())
	}
}

func TestStoreRevoke(t *testing.T) {
	s := NewStore()

	t1 := &Ticket{
		ID:        "ticket-1",
		Subject:   "agent-1",
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
	}
	s.Add(t1)

	s.Revoke("ticket-1")

	if !s.IsRevoked("ticket-1") {
		t.Error("IsRevoked(ticket-1) should be true")
	}

	active := s.ActiveTickets()
	if len(active) != 0 {
		t.Errorf("ActiveTickets() after revoke = %d, want 0", len(active))
	}

	if s.Get("ticket-1") != nil {
		t.Error("Get(ticket-1) should return nil after revocation")
	}

	if s.RevokedCount() != 1 {
		t.Errorf("RevokedCount() = %d, want 1", s.RevokedCount())
	}
}

func TestStoreCleanup(t *testing.T) {
	s := NewStore()

	// Add an already-expired ticket.
	expired := &Ticket{
		ID:        "ticket-expired",
		Subject:   "agent-old",
		IssuedAt:  time.Now().UTC().Add(-10 * time.Minute),
		ExpiresAt: time.Now().UTC().Add(-5 * time.Minute),
	}
	// Add a valid ticket.
	valid := &Ticket{
		ID:        "ticket-valid",
		Subject:   "agent-new",
		IssuedAt:  time.Now().UTC(),
		ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
	}

	s.Add(expired)
	s.Add(valid)

	cleaned := s.CleanupExpired()
	if cleaned != 1 {
		t.Errorf("CleanupExpired() removed %d, want 1", cleaned)
	}

	active := s.ActiveTickets()
	if len(active) != 1 {
		t.Errorf("ActiveTickets() after cleanup = %d, want 1", len(active))
	}
	if active[0].ID != "ticket-valid" {
		t.Errorf("remaining ticket ID = %q, want %q", active[0].ID, "ticket-valid")
	}
}
