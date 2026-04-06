package toolpolicy

import (
	"testing"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func TestDiffPoliciesDetectsAdded(t *testing.T) {
	oldRules := []ToolRule{
		{Protocol: "git", Tool: "github.list_*", Decision: "allow"},
	}
	newRules := []ToolRule{
		{Protocol: "git", Tool: "github.list_*", Decision: "allow"},
		{Protocol: "git", Tool: "github.delete_*", Decision: "block"},
	}

	diff := DiffPolicies(oldRules, newRules, nil)

	if len(diff.Added) != 1 {
		t.Fatalf("expected 1 added rule, got %d", len(diff.Added))
	}
	if diff.Added[0].Tool != "github.delete_*" {
		t.Fatalf("expected added rule tool github.delete_*, got %s", diff.Added[0].Tool)
	}
	if len(diff.Removed) != 0 {
		t.Fatalf("expected 0 removed rules, got %d", len(diff.Removed))
	}
	if len(diff.Changed) != 0 {
		t.Fatalf("expected 0 changed rules, got %d", len(diff.Changed))
	}
}

func TestDiffPoliciesDetectsRemoved(t *testing.T) {
	oldRules := []ToolRule{
		{Protocol: "git", Tool: "github.list_*", Decision: "allow"},
		{Protocol: "git", Tool: "github.delete_*", Decision: "block"},
	}
	newRules := []ToolRule{
		{Protocol: "git", Tool: "github.list_*", Decision: "allow"},
	}

	diff := DiffPolicies(oldRules, newRules, nil)

	if len(diff.Removed) != 1 {
		t.Fatalf("expected 1 removed rule, got %d", len(diff.Removed))
	}
	if diff.Removed[0].Tool != "github.delete_*" {
		t.Fatalf("expected removed rule tool github.delete_*, got %s", diff.Removed[0].Tool)
	}
	if len(diff.Added) != 0 {
		t.Fatalf("expected 0 added rules, got %d", len(diff.Added))
	}
}

func TestDiffPoliciesDetectsChanged(t *testing.T) {
	oldRules := []ToolRule{
		{Protocol: "git", Tool: "github.push", Decision: "review"},
	}
	newRules := []ToolRule{
		{Protocol: "git", Tool: "github.push", Decision: "block"},
	}

	diff := DiffPolicies(oldRules, newRules, nil)

	if len(diff.Changed) != 1 {
		t.Fatalf("expected 1 changed rule, got %d", len(diff.Changed))
	}
	if diff.Changed[0].Before.Decision != "review" {
		t.Fatalf("expected before decision review, got %s", diff.Changed[0].Before.Decision)
	}
	if diff.Changed[0].After.Decision != "block" {
		t.Fatalf("expected after decision block, got %s", diff.Changed[0].After.Decision)
	}
	if diff.Changed[0].Index != 0 {
		t.Fatalf("expected changed index 0, got %d", diff.Changed[0].Index)
	}
}

func TestDiffPoliciesShowsImpact(t *testing.T) {
	oldRules := []ToolRule{
		{Protocol: "git", Tool: "github.push", Decision: "review"},
	}
	newRules := []ToolRule{
		{Protocol: "git", Tool: "github.push", Decision: "block"},
	}

	actions := []*envelope.ActionEnvelope{
		testEnvelope(envelope.ProtocolGit, "github.push", "repo/main", envelope.CapWrite),
		testEnvelope(envelope.ProtocolGit, "github.list_repos", "org", envelope.CapRead), // no change (default both times)
	}

	diff := DiffPolicies(oldRules, newRules, actions)

	if len(diff.Impact) != 1 {
		t.Fatalf("expected 1 impacted action, got %d", len(diff.Impact))
	}
	if diff.Impact[0].Tool != "github.push" {
		t.Fatalf("expected impacted tool github.push, got %s", diff.Impact[0].Tool)
	}
	if diff.Impact[0].OldDecision != "review" {
		t.Fatalf("expected old decision review, got %s", diff.Impact[0].OldDecision)
	}
	if diff.Impact[0].NewDecision != "block" {
		t.Fatalf("expected new decision block, got %s", diff.Impact[0].NewDecision)
	}
}
