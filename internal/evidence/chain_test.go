package evidence

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func testEnv(tool string, decision envelope.Decision) *envelope.ActionEnvelope {
	return &envelope.ActionEnvelope{
		ID:                  "env-" + tool,
		Timestamp:           time.Now().UTC(),
		Actor:               envelope.ActorInfo{Type: "agent", ID: "agent-1", TenantID: "t1", SessionID: "session-1"},
		Task:                "test-task",
		Protocol:            envelope.ProtocolGit,
		Tool:                tool,
		Target:              "repo/main",
		RequestedCapability: envelope.CapWrite,
		PolicyDecision:      decision,
	}
}

func TestRecordAndRetrieve(t *testing.T) {
	chain := NewSessionChain("session-1")

	env := testEnv("github.create_pr", envelope.DecisionAllow)
	rec, err := chain.Record(env)
	if err != nil {
		t.Fatalf("record failed: %v", err)
	}
	if rec.Index != 0 {
		t.Fatalf("expected index 0, got %d", rec.Index)
	}
	if rec.Hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if rec.PreviousHash != "" {
		t.Fatal("first record should have empty previous hash")
	}

	records := chain.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
}

func TestChainLinking(t *testing.T) {
	chain := NewSessionChain("session-1")

	rec1, _ := chain.Record(testEnv("tool1", envelope.DecisionAllow))
	rec2, _ := chain.Record(testEnv("tool2", envelope.DecisionBlock))

	if rec2.PreviousHash != rec1.Hash {
		t.Fatal("second record should link to first record's hash")
	}
}

func TestHashDeterminism(t *testing.T) {
	env := testEnv("tool", envelope.DecisionAllow)
	ts := time.Now().UTC()

	rec := Record{
		Index:        0,
		Timestamp:    ts,
		Envelope:     env,
		PreviousHash: "",
	}

	hash1 := computeRecordHash(rec)
	hash2 := computeRecordHash(rec)

	if hash1 != hash2 {
		t.Fatal("same input should produce same hash")
	}
	if hash1 == "" {
		t.Fatal("hash should not be empty")
	}
}

func TestHashChangesWithDifferentInput(t *testing.T) {
	chain := NewSessionChain("s1")

	rec1, _ := chain.Record(testEnv("tool1", envelope.DecisionAllow))
	rec2, _ := chain.Record(testEnv("tool2", envelope.DecisionBlock))

	if rec1.Hash == rec2.Hash {
		t.Fatal("different inputs should produce different hashes")
	}
}

func TestSessionID(t *testing.T) {
	chain := NewSessionChain("my-session")
	if chain.SessionID() != "my-session" {
		t.Fatalf("expected my-session, got %s", chain.SessionID())
	}
}

func TestRecordCount(t *testing.T) {
	chain := NewSessionChain("s1")
	chain.Record(testEnv("t1", envelope.DecisionAllow))
	chain.Record(testEnv("t2", envelope.DecisionAllow))
	chain.Record(testEnv("t3", envelope.DecisionBlock))

	if chain.Count() != 3 {
		t.Fatalf("expected 3 records, got %d", chain.Count())
	}
}

func TestExportEmptyChain(t *testing.T) {
	chain := NewSessionChain("empty-session")
	data, err := chain.Export()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	var bundle map[string]interface{}
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if bundle["session_id"] != "empty-session" {
		t.Fatalf("expected empty-session, got %v", bundle["session_id"])
	}
	if bundle["count"].(float64) != 0 {
		t.Fatalf("expected 0 records, got %v", bundle["count"])
	}
}

func TestExportWithRecords(t *testing.T) {
	chain := NewSessionChain("export-test")
	chain.Record(testEnv("tool1", envelope.DecisionAllow))
	chain.Record(testEnv("tool2", envelope.DecisionBlock))

	data, err := chain.Export()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	var bundle map[string]interface{}
	if err := json.Unmarshal(data, &bundle); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if bundle["session_id"] != "export-test" {
		t.Fatalf("expected export-test, got %v", bundle["session_id"])
	}
	if bundle["count"].(float64) != 2 {
		t.Fatalf("expected 2 records, got %v", bundle["count"])
	}
	if bundle["last_hash"] == nil || bundle["last_hash"] == "" {
		t.Fatal("expected non-empty last_hash")
	}
	if bundle["exported_at"] == nil {
		t.Fatal("expected exported_at timestamp")
	}

	records, ok := bundle["records"].([]interface{})
	if !ok {
		t.Fatalf("expected records array, got %T", bundle["records"])
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records in array, got %d", len(records))
	}
}
