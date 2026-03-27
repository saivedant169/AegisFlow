package rollout

import (
	"database/sql"
	"fmt"
	"sort"
	"sync"
)

// MemoryStore implements Store using an in-memory map. It is intended for
// testing and development where a database is not available.
type MemoryStore struct {
	mu       sync.RWMutex
	rollouts map[string]*Rollout
}

// NewMemoryStore returns a new empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		rollouts: make(map[string]*Rollout),
	}
}

// Migrate is a no-op for the in-memory store.
func (s *MemoryStore) Migrate() error {
	return nil
}

// Create adds a rollout to the store. Returns an error if the ID already exists.
func (s *MemoryStore) Create(r *Rollout) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.rollouts[r.ID]; exists {
		return fmt.Errorf("rollout %s already exists", r.ID)
	}

	clone := *r
	s.rollouts[r.ID] = &clone
	return nil
}

// Get retrieves a rollout by ID.
func (s *MemoryStore) Get(id string) (*Rollout, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.rollouts[id]
	if !ok {
		return nil, sql.ErrNoRows
	}

	clone := *r
	return &clone, nil
}

// GetByModel returns the active rollout for a given model. Active means
// state is pending, running, or paused.
func (s *MemoryStore) GetByModel(model string) (*Rollout, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, r := range s.rollouts {
		if r.RouteModel == model && isActiveState(r.State) {
			clone := *r
			return &clone, nil
		}
	}

	return nil, sql.ErrNoRows
}

// Update replaces a rollout in the store. Returns an error if the ID does not exist.
func (s *MemoryStore) Update(r *Rollout) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.rollouts[r.ID]; !exists {
		return sql.ErrNoRows
	}

	clone := *r
	s.rollouts[r.ID] = &clone
	return nil
}

// List returns all rollouts ordered by creation time descending, limited to 50.
func (s *MemoryStore) List() ([]*Rollout, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Rollout, 0, len(s.rollouts))
	for _, r := range s.rollouts {
		clone := *r
		result = append(result, &clone)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	if len(result) > 50 {
		result = result[:50]
	}

	return result, nil
}

func isActiveState(state string) bool {
	return state == StatePending || state == StateRunning || state == StatePaused
}
