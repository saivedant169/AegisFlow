package toolpolicy

import (
	"fmt"
	"sync"
	"time"
)

// PolicyVersion represents a snapshot of the policy rules at a point in time.
type PolicyVersion struct {
	Version         int        `json:"version"`
	Timestamp       time.Time  `json:"timestamp"`
	Rules           []ToolRule `json:"rules"`
	DefaultDecision string     `json:"default_decision"`
	RuleCount       int        `json:"rule_count"`
	Source          string     `json:"source"` // "initial", "reload", "rollback"
}

// PolicyVersionStore maintains a bounded history of policy versions.
type PolicyVersionStore struct {
	mu       sync.RWMutex
	versions []PolicyVersion
	maxSize  int
}

// NewPolicyVersionStore creates a new version store with the given max history size.
func NewPolicyVersionStore(maxSize int) *PolicyVersionStore {
	if maxSize <= 0 {
		maxSize = 20
	}
	return &PolicyVersionStore{
		versions: make([]PolicyVersion, 0),
		maxSize:  maxSize,
	}
}

// Snapshot records a new policy version and returns it.
func (s *PolicyVersionStore) Snapshot(rules []ToolRule, defaultDecision, source string) PolicyVersion {
	s.mu.Lock()
	defer s.mu.Unlock()

	ver := PolicyVersion{
		Version:         len(s.versions) + 1,
		Timestamp:       time.Now().UTC(),
		Rules:           make([]ToolRule, len(rules)),
		DefaultDecision: defaultDecision,
		RuleCount:       len(rules),
		Source:          source,
	}
	copy(ver.Rules, rules)

	s.versions = append(s.versions, ver)
	if len(s.versions) > s.maxSize {
		s.versions = s.versions[len(s.versions)-s.maxSize:]
	}

	return ver
}

// List returns all stored versions.
func (s *PolicyVersionStore) List() []PolicyVersion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]PolicyVersion, len(s.versions))
	copy(result, s.versions)
	return result
}

// Get returns a specific version by number, or an error if not found.
func (s *PolicyVersionStore) Get(version int) (*PolicyVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, v := range s.versions {
		if v.Version == version {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("version %d not found", version)
}

// Current returns the most recent version, or nil if empty.
func (s *PolicyVersionStore) Current() *PolicyVersion {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.versions) == 0 {
		return nil
	}
	v := s.versions[len(s.versions)-1]
	return &v
}

// Len returns the number of stored versions.
func (s *PolicyVersionStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.versions)
}
