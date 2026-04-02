package audit

import (
	"sync"
)

type MemoryStore struct {
	mu      sync.RWMutex
	entries []Entry
	nextID  int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{nextID: 1}
}

func (s *MemoryStore) Migrate() error { return nil }

func (s *MemoryStore) Insert(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.ID = s.nextID
	s.nextID++
	s.entries = append(s.entries, entry)
	return nil
}

func (s *MemoryStore) LastHash() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.entries) == 0 {
		return "", nil
	}
	return s.entries[len(s.entries)-1].EntryHash, nil
}

func (s *MemoryStore) Query(filters QueryFilters) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Entry, 0, len(s.entries))
	for _, entry := range s.entries {
		if filters.Actor != "" && entry.Actor != filters.Actor {
			continue
		}
		if filters.ActorRole != "" && entry.ActorRole != filters.ActorRole {
			continue
		}
		if filters.Action != "" && entry.Action != filters.Action {
			continue
		}
		if filters.TenantID != "" && entry.TenantID != filters.TenantID {
			continue
		}
		if !filters.From.IsZero() && entry.Timestamp.Before(filters.From) {
			continue
		}
		if !filters.To.IsZero() && entry.Timestamp.After(filters.To) {
			continue
		}
		result = append(result, entry)
	}
	if filters.Limit > 0 && len(result) > filters.Limit {
		result = result[:filters.Limit]
	}
	return result, nil
}
