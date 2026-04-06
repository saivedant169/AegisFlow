package toolpolicy

import (
	"testing"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func TestEvaluateWithTraceShowsMatchedRule(t *testing.T) {
	rules := []ToolRule{
		{Protocol: "git", Tool: "github.list_*", Decision: "allow"},
		{Protocol: "git", Tool: "github.delete_repo", Decision: "block"},
	}
	engine := NewEngine(rules, "review")

	env := testEnvelope(envelope.ProtocolGit, "github.delete_repo", "myrepo", envelope.CapDelete)
	trace := engine.EvaluateWithTrace(env)

	if trace.Decision != "block" {
		t.Fatalf("expected block, got %s", trace.Decision)
	}
	if trace.MatchedIndex != 1 {
		t.Fatalf("expected matched index 1, got %d", trace.MatchedIndex)
	}
	if trace.MatchedRule == nil {
		t.Fatal("expected matched rule to be set")
	}
	if trace.MatchedRule.Tool != "github.delete_repo" {
		t.Fatalf("expected matched rule tool github.delete_repo, got %s", trace.MatchedRule.Tool)
	}
	if trace.DefaultUsed {
		t.Fatal("expected default not used")
	}
}

func TestEvaluateWithTraceShowsDefaultWhenNoMatch(t *testing.T) {
	rules := []ToolRule{
		{Protocol: "git", Tool: "github.list_*", Decision: "allow"},
	}
	engine := NewEngine(rules, "block")

	env := testEnvelope(envelope.ProtocolShell, "shell.exec", "/bin/bash", envelope.CapExecute)
	trace := engine.EvaluateWithTrace(env)

	if trace.Decision != "block" {
		t.Fatalf("expected block (default), got %s", trace.Decision)
	}
	if !trace.DefaultUsed {
		t.Fatal("expected default used to be true")
	}
	if trace.MatchedIndex != -1 {
		t.Fatalf("expected matched index -1, got %d", trace.MatchedIndex)
	}
	if trace.MatchedRule != nil {
		t.Fatal("expected no matched rule")
	}
	if trace.RulesChecked != 1 {
		t.Fatalf("expected 1 rule checked, got %d", trace.RulesChecked)
	}
}

func TestEvaluateWithTraceShowsAllCheckedRules(t *testing.T) {
	rules := []ToolRule{
		{Protocol: "git", Tool: "github.list_*", Decision: "allow"},
		{Protocol: "sql", Tool: "postgres.*", Decision: "review"},
		{Protocol: "shell", Tool: "shell.exec", Decision: "block"},
	}
	engine := NewEngine(rules, "review")

	env := testEnvelope(envelope.ProtocolShell, "shell.exec", "/bin/bash", envelope.CapExecute)
	trace := engine.EvaluateWithTrace(env)

	if trace.Decision != "block" {
		t.Fatalf("expected block, got %s", trace.Decision)
	}
	// Should have checked all 3 rules; third matched.
	if len(trace.CheckTrace) != 3 {
		t.Fatalf("expected 3 check trace entries, got %d", len(trace.CheckTrace))
	}
	if trace.CheckTrace[0].Matched {
		t.Fatal("rule 0 should not have matched")
	}
	if trace.CheckTrace[1].Matched {
		t.Fatal("rule 1 should not have matched")
	}
	if !trace.CheckTrace[2].Matched {
		t.Fatal("rule 2 should have matched")
	}
	if trace.RulesChecked != 3 {
		t.Fatalf("expected 3 rules checked, got %d", trace.RulesChecked)
	}
}

func TestEvaluateWithTraceFailReasons(t *testing.T) {
	rules := []ToolRule{
		{Protocol: "git", Tool: "github.list_*", Capability: "read", Decision: "allow"},
		{Protocol: "git", Tool: "github.delete_*", Target: "production/*", Decision: "block"},
	}
	engine := NewEngine(rules, "review")

	// This envelope won't match rule 0 (tool glob mismatch) and won't match
	// rule 1 (target glob mismatch), so default is used.
	env := testEnvelope(envelope.ProtocolGit, "github.delete_repo", "staging/myrepo", envelope.CapDelete)
	trace := engine.EvaluateWithTrace(env)

	if trace.Decision != "review" {
		t.Fatalf("expected review (default), got %s", trace.Decision)
	}

	// Rule 0: tool glob should fail.
	if trace.CheckTrace[0].FailReason != "tool glob no match" {
		t.Fatalf("expected 'tool glob no match' for rule 0, got %q", trace.CheckTrace[0].FailReason)
	}

	// Rule 1: target glob should fail.
	if trace.CheckTrace[1].FailReason != "target glob no match" {
		t.Fatalf("expected 'target glob no match' for rule 1, got %q", trace.CheckTrace[1].FailReason)
	}

	// Also verify protocol mismatch and capability mismatch reasons.
	rules2 := []ToolRule{
		{Protocol: "sql", Tool: "*", Decision: "allow"},
		{Protocol: "git", Tool: "*", Capability: "read", Decision: "allow"},
	}
	engine2 := NewEngine(rules2, "block")

	env2 := testEnvelope(envelope.ProtocolGit, "github.push", "repo", envelope.CapWrite)
	trace2 := engine2.EvaluateWithTrace(env2)

	if trace2.CheckTrace[0].FailReason != "protocol mismatch" {
		t.Fatalf("expected 'protocol mismatch' for rule 0, got %q", trace2.CheckTrace[0].FailReason)
	}
	if trace2.CheckTrace[1].FailReason != "capability mismatch" {
		t.Fatalf("expected 'capability mismatch' for rule 1, got %q", trace2.CheckTrace[1].FailReason)
	}
}
