package toolpolicy

import "testing"

func TestSnapshotAndList(t *testing.T) {
	store := NewPolicyVersionStore(5)
	rules := []ToolRule{{Protocol: "shell", Tool: "rm", Decision: "block"}}

	v := store.Snapshot(rules, "review", "initial")
	if v.Version != 1 {
		t.Fatalf("expected version 1, got %d", v.Version)
	}

	list := store.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 version, got %d", len(list))
	}
}

func TestSnapshotMaxSize(t *testing.T) {
	store := NewPolicyVersionStore(3)
	for i := 0; i < 5; i++ {
		store.Snapshot([]ToolRule{}, "allow", "reload")
	}
	if store.Len() != 3 {
		t.Fatalf("expected 3 versions (max), got %d", store.Len())
	}
}

func TestGetVersion(t *testing.T) {
	store := NewPolicyVersionStore(10)
	store.Snapshot([]ToolRule{{Tool: "v1"}}, "allow", "initial")
	store.Snapshot([]ToolRule{{Tool: "v2"}}, "block", "reload")

	v, err := store.Get(1)
	if err != nil {
		t.Fatal(err)
	}
	if v.Rules[0].Tool != "v1" {
		t.Fatalf("expected v1 rules, got %s", v.Rules[0].Tool)
	}
}

func TestGetVersionNotFound(t *testing.T) {
	store := NewPolicyVersionStore(10)
	_, err := store.Get(99)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCurrent(t *testing.T) {
	store := NewPolicyVersionStore(10)
	if store.Current() != nil {
		t.Fatal("expected nil current on empty store")
	}
	store.Snapshot([]ToolRule{{Tool: "first"}}, "allow", "initial")
	store.Snapshot([]ToolRule{{Tool: "second"}}, "block", "reload")

	cur := store.Current()
	if cur.Rules[0].Tool != "second" {
		t.Fatalf("expected second, got %s", cur.Rules[0].Tool)
	}
}

func TestReplaceRules(t *testing.T) {
	rules := []ToolRule{{Protocol: "shell", Tool: "rm", Decision: "block"}}
	engine := NewEngine(rules, "allow")

	newRules := []ToolRule{{Protocol: "git", Tool: "push", Decision: "review"}}
	engine.ReplaceRules(newRules, "block")

	got := engine.Rules()
	if len(got) != 1 || got[0].Tool != "push" {
		t.Fatalf("expected replaced rules, got %v", got)
	}
	if engine.DefaultDecision() != "block" {
		t.Fatalf("expected block default, got %s", engine.DefaultDecision())
	}
}
