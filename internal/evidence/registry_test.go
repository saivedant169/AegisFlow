package evidence

import (
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

func TestChainRegistry_SplitsBySession(t *testing.T) {
	r := NewChainRegistry(nil)
	defer r.Close()

	r.Record(envForSession("s1", "e1"))
	r.Record(envForSession("s1", "e2"))
	r.Record(envForSession("s2", "e3"))

	if got := len(r.get("s1").Records()); got != 2 {
		t.Fatalf("session s1 should have 2 records, got %d", got)
	}
	if got := len(r.get("s2").Records()); got != 1 {
		t.Fatalf("session s2 should have 1 record, got %d", got)
	}
	if r.get("missing") != nil {
		t.Fatal("unknown session should be nil")
	}
}

func TestChainRegistry_SignsRecords(t *testing.T) {
	key := []byte("reg-key")
	r := NewChainRegistry(key)
	defer r.Close()
	r.Record(envForSession("s1", "e1"))

	recs := r.get("s1").Records()
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
	r.Record(envForSession("s1", "e1"))
	a := NewRegistryAdminAdapter(r)

	if _, err := a.ExportSession("s1"); err != nil {
		t.Fatalf("export: %v", err)
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
	list, ok := a.ListSessions().([]SessionManifest)
	if !ok || len(list) != 1 {
		t.Fatalf("expected 1 session manifest, got %+v", a.ListSessions())
	}
}
