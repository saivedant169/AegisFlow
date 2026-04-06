package manifest

import (
	"fmt"
	"sync"
	"time"
)

// Store is an in-memory store for active TaskManifests.
type Store struct {
	mu        sync.RWMutex
	manifests map[string]*TaskManifest
	// driftEvents maps manifest ID to its drift events.
	driftEvents map[string][]DriftEvent
}

// NewStore creates a new manifest store.
func NewStore() *Store {
	return &Store{
		manifests:   make(map[string]*TaskManifest),
		driftEvents: make(map[string][]DriftEvent),
	}
}

// Register adds a manifest to the store. It computes and sets the manifest hash.
func (s *Store) Register(m *TaskManifest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if m.ID == "" {
		return fmt.Errorf("manifest ID is required")
	}
	if m.TaskID == "" {
		return fmt.Errorf("task_id is required")
	}

	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	m.ManifestHash = m.ComputeHash()
	m.Active = true
	s.manifests[m.ID] = m
	return nil
}

// Get returns a manifest by ID, or an error if not found.
func (s *Store) Get(id string) (*TaskManifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.manifests[id]
	if !ok {
		return nil, fmt.Errorf("manifest %q not found", id)
	}
	return m, nil
}

// GetByTaskID returns the first active manifest matching the given task ID.
func (s *Store) GetByTaskID(taskID string) (*TaskManifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, m := range s.manifests {
		if m.TaskID == taskID && m.Active {
			return m, nil
		}
	}
	return nil, fmt.Errorf("no active manifest for task %q", taskID)
}

// Deactivate marks a manifest as inactive.
func (s *Store) Deactivate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := s.manifests[id]
	if !ok {
		return fmt.Errorf("manifest %q not found", id)
	}
	m.Active = false
	return nil
}

// List returns all manifests (active and inactive).
func (s *Store) List() []*TaskManifest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*TaskManifest, 0, len(s.manifests))
	for _, m := range s.manifests {
		result = append(result, m)
	}
	return result
}

// ListActive returns only active manifests.
func (s *Store) ListActive() []*TaskManifest {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*TaskManifest
	for _, m := range s.manifests {
		if m.Active {
			result = append(result, m)
		}
	}
	return result
}

// RecordDrift appends drift events for a manifest.
func (s *Store) RecordDrift(manifestID string, events []DriftEvent) {
	if len(events) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.driftEvents[manifestID] = append(s.driftEvents[manifestID], events...)
}

// GetDrift returns all recorded drift events for a manifest.
func (s *Store) GetDrift(manifestID string) []DriftEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := s.driftEvents[manifestID]
	if events == nil {
		return []DriftEvent{}
	}
	result := make([]DriftEvent, len(events))
	copy(result, events)
	return result
}
