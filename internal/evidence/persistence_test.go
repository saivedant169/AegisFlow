package evidence

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/resource"
	"github.com/saivedant169/AegisFlow/internal/state"
)

func persistentEvidenceEnv(sessionID, id string) *envelope.ActionEnvelope {
	return &envelope.ActionEnvelope{
		ID:        id,
		Timestamp: time.Now().UTC(),
		Actor: envelope.ActorInfo{
			Type:      "agent",
			ID:        "agent-1",
			SessionID: sessionID,
			TenantID:  "tenant-1",
		},
		Task:                "change request",
		Protocol:            envelope.ProtocolMCP,
		Tool:                "repo.open_change",
		Target:              "acme/widgets",
		Parameters:          map[string]any{"head": "fix/state"},
		RequestedCapability: envelope.CapWrite,
		PolicyDecision:      envelope.DecisionReview,
	}
}

func TestPersistentChainRegistryRestoresAndAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	key := []byte("persistent-evidence-key")

	firstState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewPersistentChainRegistry(key, firstState.DB())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Record(persistentEvidenceEnv("session-1", "record-1")); err != nil {
		t.Fatalf("record first action: %v", err)
	}
	first.Close()
	if err := firstState.Close(); err != nil {
		t.Fatal(err)
	}

	secondState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func(secondState *state.SQLite) {
		err := secondState.Close()
		if err != nil {
			t.Fatal(err)
		}
	}(secondState)
	second, err := NewPersistentChainRegistry(key, secondState.DB())
	if err != nil {
		t.Fatalf("restore registry: %v", err)
	}
	defer second.Close()

	chain := mustGetChain(t, second, "session-1")
	if chain == nil || chain.Count() != 1 {
		t.Fatalf("restored record count = %v, want 1", chain)
	}
	if _, err := second.Record(persistentEvidenceEnv("session-1", "record-2")); err != nil {
		t.Fatalf("append restored chain: %v", err)
	}
	result := VerifySignatures(chain.Records(), key)
	if !result.Valid || result.TotalRecords != 2 {
		t.Fatalf("restored chain verification failed: %+v", result)
	}
}

func TestPersistentChainRegistryRejectsWrongKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	firstState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewPersistentChainRegistry([]byte("first-key"), firstState.DB())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Record(persistentEvidenceEnv("session-1", "record-1")); err != nil {
		t.Fatal(err)
	}
	first.Close()
	if err := firstState.Close(); err != nil {
		t.Fatal(err)
	}

	secondState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func(secondState *state.SQLite) {
		err := secondState.Close()
		if err != nil {
			t.Fatal(err)
		}
	}(secondState)
	if _, err := NewPersistentChainRegistry([]byte("different-key"), secondState.DB()); err == nil {
		t.Fatal("expected restored evidence to reject a different signing key")
	}
}

func TestPersistentChainRegistryRequiresSigningKey(t *testing.T) {
	store, err := state.OpenSQLite(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func(store *state.SQLite) {
		err := store.Close()
		if err != nil {
			t.Fatal(err)
		}
	}(store)
	if _, err := NewPersistentChainRegistry(nil, store.DB()); err == nil {
		t.Fatal("persistent evidence accepted an empty signing key")
	}
}

func TestPersistentChainRegistryRejectsTamperedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	key := []byte("tamper-test-key")
	store, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewPersistentChainRegistry(key, store.DB())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Record(persistentEvidenceEnv("session-1", "record-1")); err != nil {
		t.Fatal(err)
	}
	registry.Close()

	var data []byte
	if err := store.DB().QueryRow(
		"SELECT record_json FROM evidence_records WHERE session_id = ? AND record_index = 0",
		"session-1",
	).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	record.Envelope.Target = "acme/other"
	tampered, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().Exec(
		"UPDATE evidence_records SET record_json = ? WHERE session_id = ? AND record_index = 0",
		tampered, "session-1",
	); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func(reopened *state.SQLite) {
		err := reopened.Close()
		if err != nil {
			t.Fatal(err)
		}
	}(reopened)
	if _, err := NewPersistentChainRegistry(key, reopened.DB()); err == nil {
		t.Fatal("expected tampered evidence to stop registry restoration")
	}
}

func TestPersistentChainRegistryAuthenticatesFullStoredRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	key := []byte("full-record-key")
	store, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewPersistentChainRegistry(key, store.DB())
	if err != nil {
		t.Fatal(err)
	}
	env := persistentEvidenceEnv("session-1", "record-1")
	env.Resource = &resource.Resource{
		Type: resource.ResourceRepo,
		Path: []string{"acme", "widgets"},
	}
	if _, err := registry.Record(env); err != nil {
		t.Fatal(err)
	}
	registry.Close()

	var data []byte
	if err := store.DB().QueryRow(
		"SELECT record_json FROM evidence_records WHERE session_id = ? AND record_index = 0",
		"session-1",
	).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	record.Envelope.Resource.Path[1] = "altered"
	tampered, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB().Exec(
		"UPDATE evidence_records SET record_json = ? WHERE session_id = ? AND record_index = 0",
		tampered, "session-1",
	); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func(reopened *state.SQLite) {
		err := reopened.Close()
		if err != nil {
			t.Fatal(err)
		}
	}(reopened)
	if _, err := NewPersistentChainRegistry(key, reopened.DB()); err == nil {
		t.Fatal("unsigned resource edit passed storage signature check")
	}
}

func TestPersistentChainRegistryRejectsSessionRelocation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	key := []byte("session-binding-key")
	store, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewPersistentChainRegistry(key, store.DB())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Record(persistentEvidenceEnv("session-1", "record-1")); err != nil {
		t.Fatal(err)
	}
	registry.Close()
	if _, err := store.DB().Exec(
		"UPDATE evidence_records SET session_id = ? WHERE session_id = ?",
		"session-2", "session-1",
	); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func(reopened *state.SQLite) {
		err := reopened.Close()
		if err != nil {
			t.Fatal(err)
		}
	}(reopened)
	if _, err := NewPersistentChainRegistry(key, reopened.DB()); err == nil {
		t.Fatal("evidence row moved to another session without detection")
	}
}

func TestPersistentChainRecordFailureDoesNotMutateMemory(t *testing.T) {
	store, err := state.OpenSQLite(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewPersistentChainRegistry([]byte("failure-key"), store.DB())
	if err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	if _, err := registry.Record(persistentEvidenceEnv("session-1", "record-1")); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := registry.Record(persistentEvidenceEnv("session-1", "record-2")); err == nil {
		t.Fatal("expected evidence append to fail after database close")
	}
	registry.mu.Lock()
	chain := registry.chains["session-1"].chain
	registry.mu.Unlock()
	if chain == nil {
		t.Fatal("expected cached session chain")
	}
	if chain.Count() != 1 {
		t.Fatalf("failed append changed in-memory chain count to %d", chain.Count())
	}
}

func TestSessionChainSnapshotsEnvelope(t *testing.T) {
	chain := NewSignedSessionChain("session-1", []byte("snapshot-key"))
	env := persistentEvidenceEnv("session-1", "record-1")
	if _, err := chain.Record(env); err != nil {
		t.Fatal(err)
	}
	env.Target = "changed-after-record"

	records := chain.Records()
	if records[0].Envelope.Target != "acme/widgets" {
		t.Fatalf("stored target changed to %q", records[0].Envelope.Target)
	}
	if result := VerifySignatures(records, []byte("snapshot-key")); !result.Valid {
		t.Fatalf("snapshot verification failed: %+v", result)
	}
}

func TestSessionChainSnapshotsNestedParametersAndResource(t *testing.T) {
	chain := NewSignedSessionChain("session-1", []byte("nested-snapshot-key"))
	env := persistentEvidenceEnv("session-1", "record-1")
	nested := map[string]any{"labels": []any{"safe", map[string]any{"branch": "main"}}}
	env.Parameters["nested"] = nested
	env.Resource = &resource.Resource{
		Type:       resource.ResourceRepo,
		Path:       []string{"acme", "widgets"},
		Properties: map[string]string{"branch": "main"},
	}
	if _, err := chain.Record(env); err != nil {
		t.Fatal(err)
	}
	nested["labels"].([]any)[1].(map[string]any)["branch"] = "changed"
	env.Resource.Path[1] = "changed"
	env.Resource.Properties["branch"] = "changed"

	record := chain.Records()[0]
	storedNested := record.Envelope.Parameters["nested"].(map[string]any)
	storedBranch := storedNested["labels"].([]any)[1].(map[string]any)["branch"]
	if storedBranch != "main" {
		t.Fatalf("nested parameter changed to %v", storedBranch)
	}
	if record.Envelope.Resource.Path[1] != "widgets" {
		t.Fatalf("resource path changed to %q", record.Envelope.Resource.Path[1])
	}
	if record.Envelope.Resource.Properties["branch"] != "main" {
		t.Fatalf("resource property changed to %q", record.Envelope.Resource.Properties["branch"])
	}
}
