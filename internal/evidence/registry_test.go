package evidence

import (
	"encoding/json"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

var _ Recorder = (*ChainRegistry)(nil)
var _ Recorder = (*SessionChain)(nil)

func envForSession(session, id string) *envelope.ActionEnvelope {
	return &envelope.ActionEnvelope{
		ID:                  id,
		Actor:               envelope.ActorInfo{Type: "agent", ID: "a", SessionID: session},
		Tool:                "github.create_pull_request",
		Target:              "acme/widgets",
		PolicyDecision:      envelope.DecisionReview,
		RequestedCapability: envelope.CapWrite,
	}
}

func mustGetChain(t *testing.T, registry *ChainRegistry, sessionID string) *SessionChain {
	t.Helper()
	chain, err := registry.get(sessionID)
	if err != nil {
		t.Fatalf("get session %q: %v", sessionID, err)
	}
	return chain
}

func TestChainRegistry_SplitsBySession(t *testing.T) {
	r := NewChainRegistry(nil)
	defer r.Close()

	_, err := r.Record(envForSession("s1", "e1"))
	if err != nil {
		return
	}
	_, err = r.Record(envForSession("s1", "e2"))
	if err != nil {
		return
	}
	_, err = r.Record(envForSession("s2", "e3"))
	if err != nil {
		return
	}

	if got := len(mustGetChain(t, r, "s1").Records()); got != 2 {
		t.Fatalf("session s1 should have 2 records, got %d", got)
	}
	if got := len(mustGetChain(t, r, "s2").Records()); got != 1 {
		t.Fatalf("session s2 should have 1 record, got %d", got)
	}
	if mustGetChain(t, r, "missing") != nil {
		t.Fatal("unknown session should be nil")
	}
}

func TestChainRegistry_SignsRecords(t *testing.T) {
	key := []byte("reg-key")
	r := NewChainRegistry(key)
	defer r.Close()
	_, err := r.Record(envForSession("s1", "e1"))
	if err != nil {
		return
	}

	recs := mustGetChain(t, r, "s1").Records()
	if recs[0].Signature == "" {
		t.Fatal("registry with a key should sign records")
	}
	if res := VerifySignatures(recs, key); !res.Valid {
		t.Fatalf("expected valid signatures, got %s", res.Message)
	}
}

func TestRegistryAdminAdapter(t *testing.T) {
	r := NewChainRegistry([]byte("k"))
	defer r.Close()
	_, err := r.Record(envForSession("s1", "e1"))
	if err != nil {
		return
	}
	a := NewRegistryAdminAdapter(r)

	exported, err := a.ExportSession("s1")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	raw, err := json.Marshal(exported)
	if err != nil {
		t.Fatalf("marshal export: %v", err)
	}
	var bundle struct {
		SessionID string   `json:"session_id"`
		Records   []Record `json:"records"`
	}
	if err := json.Unmarshal(raw, &bundle); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if bundle.SessionID != "s1" || len(bundle.Records) != 1 {
		t.Fatalf("unexpected export: %s", raw)
	}
	if _, err := a.ExportSession("nope"); err == nil {
		t.Fatal("expected error for unknown session")
	}
	v, err := a.VerifySession("s1")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if res, ok := v.(VerifyResult); !ok || !res.Valid {
		t.Fatalf("expected valid verify result, got %+v", v)
	}
	listed, err := a.ListSessions()
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	list, ok := listed.([]SessionManifest)
	if !ok || len(list) != 1 {
		t.Fatalf("expected 1 session manifest, got %+v", listed)
	}
}

func TestRegistryRejectsCrossTenantSessionAppend(t *testing.T) {
	registry := NewChainRegistry([]byte("tenant-test-key"))
	defer registry.Close()
	first := envelope.NewEnvelope(envelope.ActorInfo{ID: "agent-a", TenantID: "tenant-a", SessionID: "shared-label"}, "task", envelope.ProtocolMCP, "repo.read", "repo", envelope.CapRead)
	if _, err := registry.Record(first); err != nil {
		t.Fatal(err)
	}
	second := envelope.NewEnvelope(envelope.ActorInfo{ID: "agent-b", TenantID: "tenant-b", SessionID: "shared-label"}, "task", envelope.ProtocolMCP, "repo.read", "repo", envelope.CapRead)
	if _, err := registry.Record(second); err == nil {
		t.Fatal("another tenant appended to owned evidence session")
	}
	chain, err := registry.get("shared-label")
	if err != nil {
		t.Fatal(err)
	}
	if chain.Count() != 1 {
		t.Fatal("rejected action changed evidence")
	}
	if !VerifySignatures(chain.Records(), []byte("tenant-test-key")).Valid {
		t.Fatal("rejection damaged evidence integrity")
	}
}
