package capability

import (
	"sync"
)

// Store is an in-memory store for issued tickets and the revocation list.
type Store struct {
	mu      sync.RWMutex
	active  map[string]*Ticket // ticketID -> Ticket
	revoked map[string]bool    // ticketID -> true
}

// NewStore creates an empty ticket store.
func NewStore() *Store {
	return &Store{
		active:  make(map[string]*Ticket),
		revoked: make(map[string]bool),
	}
}

// Add stores an issued ticket.
func (s *Store) Add(t *Ticket) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active[t.ID] = t
}

// Get returns a ticket by ID, or nil if not found.
func (s *Store) Get(id string) *Ticket {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active[id]
}

// ActiveTickets returns all non-expired, non-revoked tickets.
func (s *Store) ActiveTickets() []*Ticket {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*Ticket
	for _, t := range s.active {
		if !t.Expired() && !s.revoked[t.ID] {
			result = append(result, t)
		}
	}
	return result
}

// Revoke adds a ticket ID to the revocation list and removes it from active.
func (s *Store) Revoke(ticketID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked[ticketID] = true
	delete(s.active, ticketID)
}

// IsRevoked returns true if the ticket has been revoked.
func (s *Store) IsRevoked(ticketID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revoked[ticketID]
}

// CleanupExpired removes all expired tickets from the active set.
func (s *Store) CleanupExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for id, t := range s.active {
		if t.Expired() {
			delete(s.active, id)
			count++
		}
	}
	return count
}

// ActiveCount returns the number of active (non-expired) tickets.
func (s *Store) ActiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, t := range s.active {
		if !t.Expired() {
			count++
		}
	}
	return count
}

// RevokedCount returns the number of revoked ticket IDs tracked.
func (s *Store) RevokedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.revoked)
}
