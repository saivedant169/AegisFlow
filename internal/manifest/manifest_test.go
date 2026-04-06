package manifest

import (
	"testing"
	"time"
)

func TestManifestHashDeterministic(t *testing.T) {
	m := &TaskManifest{
		ID:               "m-1",
		TaskID:           "TICKET-100",
		AllowedTools:     []string{"github.*"},
		AllowedResources: []string{"repos/*"},
		AllowedProtocols: []string{"git"},
		AllowedVerbs:     []string{"read"},
		MaxActions:       10,
		RiskTier:         "low",
	}

	hash1 := m.ComputeHash()
	hash2 := m.ComputeHash()

	if hash1 != hash2 {
		t.Errorf("hash should be deterministic: %s != %s", hash1, hash2)
	}
	if len(hash1) != 64 { // sha256 hex length
		t.Errorf("expected 64 char hex hash, got %d chars", len(hash1))
	}
}

func TestManifestHashChanges(t *testing.T) {
	m1 := &TaskManifest{
		TaskID:       "TICKET-100",
		AllowedTools: []string{"github.*"},
		MaxActions:   10,
		RiskTier:     "low",
	}

	m2 := &TaskManifest{
		TaskID:       "TICKET-100",
		AllowedTools: []string{"github.*", "slack.*"},
		MaxActions:   10,
		RiskTier:     "low",
	}

	hash1 := m1.ComputeHash()
	hash2 := m2.ComputeHash()

	if hash1 == hash2 {
		t.Error("different manifests should produce different hashes")
	}
}

func TestStoreRegisterAndGet(t *testing.T) {
	s := NewStore()

	m := &TaskManifest{
		ID:           "m-1",
		TaskID:       "TICKET-100",
		AllowedTools: []string{"github.*"},
	}

	if err := s.Register(m); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got, err := s.Get("m-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.TaskID != "TICKET-100" {
		t.Errorf("expected TaskID TICKET-100, got %s", got.TaskID)
	}
	if !got.Active {
		t.Error("expected manifest to be active after registration")
	}
	if got.ManifestHash == "" {
		t.Error("expected ManifestHash to be set after registration")
	}
	if got.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Not found
	_, err = s.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent manifest")
	}
}

func TestStoreDeactivate(t *testing.T) {
	s := NewStore()
	m := &TaskManifest{
		ID:     "m-1",
		TaskID: "TICKET-100",
	}
	s.Register(m)

	if err := s.Deactivate("m-1"); err != nil {
		t.Fatalf("Deactivate failed: %v", err)
	}

	got, _ := s.Get("m-1")
	if got.Active {
		t.Error("expected manifest to be inactive after deactivation")
	}

	// Deactivate nonexistent
	if err := s.Deactivate("nonexistent"); err == nil {
		t.Error("expected error for nonexistent manifest")
	}
}

func TestStoreList(t *testing.T) {
	s := NewStore()

	s.Register(&TaskManifest{ID: "m-1", TaskID: "T-1"})
	s.Register(&TaskManifest{ID: "m-2", TaskID: "T-2"})
	s.Register(&TaskManifest{ID: "m-3", TaskID: "T-3"})

	all := s.List()
	if len(all) != 3 {
		t.Errorf("expected 3 manifests, got %d", len(all))
	}

	// Deactivate one and check ListActive
	s.Deactivate("m-2")
	active := s.ListActive()
	if len(active) != 2 {
		t.Errorf("expected 2 active manifests, got %d", len(active))
	}
}

func TestStoreRegisterValidation(t *testing.T) {
	s := NewStore()

	// Missing ID
	err := s.Register(&TaskManifest{TaskID: "T-1"})
	if err == nil {
		t.Error("expected error for missing ID")
	}

	// Missing TaskID
	err = s.Register(&TaskManifest{ID: "m-1"})
	if err == nil {
		t.Error("expected error for missing TaskID")
	}
}

func TestStoreDriftRecording(t *testing.T) {
	s := NewStore()
	s.Register(&TaskManifest{ID: "m-1", TaskID: "T-1"})

	events := []DriftEvent{
		{Type: DriftUnexpectedTool, ManifestID: "m-1", Message: "bad tool", Timestamp: time.Now()},
		{Type: DriftUnexpectedVerb, ManifestID: "m-1", Message: "bad verb", Timestamp: time.Now()},
	}
	s.RecordDrift("m-1", events)

	got := s.GetDrift("m-1")
	if len(got) != 2 {
		t.Errorf("expected 2 drift events, got %d", len(got))
	}

	// Empty drift for unknown manifest
	got2 := s.GetDrift("nonexistent")
	if len(got2) != 0 {
		t.Errorf("expected 0 drift events for unknown manifest, got %d", len(got2))
	}
}
