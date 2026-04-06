package main

import (
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

func TestFormatSimulateOutput_Allowed(t *testing.T) {
	r := &simulateResponse{
		Action:   "github.list_repos",
		Decision: "allow",
		Trace: &toolpolicy.PolicyDecisionTrace{
			Decision:     "allow",
			MatchedIndex: 0,
			RulesChecked: 1,
			MatchedRule: &toolpolicy.ToolRule{
				Protocol: "git", Tool: "github.list_*", Decision: "allow",
			},
			CheckTrace: []toolpolicy.RuleCheckStep{
				{RuleIndex: 0, Rule: toolpolicy.ToolRule{Protocol: "git", Tool: "github.list_*", Decision: "allow"}, Matched: true},
			},
		},
	}

	out := formatSimulateOutput(r)
	if !strings.Contains(out, "ALLOWED") {
		t.Fatalf("expected ALLOWED in output, got:\n%s", out)
	}
	if !strings.Contains(out, "HIT") {
		t.Fatalf("expected HIT in trace output, got:\n%s", out)
	}
	if !strings.Contains(out, "Matched rule:   #0") {
		t.Fatalf("expected matched rule #0 in output, got:\n%s", out)
	}
}

func TestFormatSimulateOutput_Blocked(t *testing.T) {
	r := &simulateResponse{
		Action:   "rm",
		Decision: "block",
		Trace: &toolpolicy.PolicyDecisionTrace{
			Decision:     "block",
			MatchedIndex: 0,
			RulesChecked: 1,
			MatchedRule: &toolpolicy.ToolRule{
				Protocol: "shell", Tool: "rm", Decision: "block",
			},
			CheckTrace: []toolpolicy.RuleCheckStep{
				{RuleIndex: 0, Rule: toolpolicy.ToolRule{Protocol: "shell", Tool: "rm", Decision: "block"}, Matched: true},
			},
		},
	}

	out := formatSimulateOutput(r)
	if !strings.Contains(out, "BLOCKED") {
		t.Fatalf("expected BLOCKED in output, got:\n%s", out)
	}
}

func TestFormatSimulateOutput_Default(t *testing.T) {
	r := &simulateResponse{
		Action:   "unknown.tool",
		Decision: "review",
		Trace: &toolpolicy.PolicyDecisionTrace{
			Decision:     "review",
			MatchedIndex: -1,
			RulesChecked: 1,
			DefaultUsed:  true,
			CheckTrace: []toolpolicy.RuleCheckStep{
				{RuleIndex: 0, Rule: toolpolicy.ToolRule{Protocol: "git", Tool: "github.list_*", Decision: "allow"}, FailReason: "protocol mismatch"},
			},
		},
	}

	out := formatSimulateOutput(r)
	if !strings.Contains(out, "REVIEW REQUIRED") {
		t.Fatalf("expected REVIEW REQUIRED in output, got:\n%s", out)
	}
	if !strings.Contains(out, "(default)") {
		t.Fatalf("expected (default) in output, got:\n%s", out)
	}
	if !strings.Contains(out, "protocol mismatch") {
		t.Fatalf("expected fail reason in output, got:\n%s", out)
	}
}

func TestFormatSimulateOutput_Local(t *testing.T) {
	r := &simulateResponse{
		Action:   "test",
		Decision: "block",
		Local:    true,
	}

	out := formatSimulateOutput(r)
	if !strings.Contains(out, "local evaluation") {
		t.Fatalf("expected local evaluation notice, got:\n%s", out)
	}
}

func TestFormatDiffOutput_Added(t *testing.T) {
	diff := &toolpolicy.DiffResult{
		Added: []toolpolicy.ToolRule{
			{Protocol: "git", Tool: "github.delete_*", Decision: "block"},
		},
		Removed: []toolpolicy.ToolRule{},
		Changed: []toolpolicy.RuleChange{},
		Impact:  []toolpolicy.ImpactedAction{},
	}

	out := formatDiffOutput(diff)
	if !strings.Contains(out, "Added rules:") {
		t.Fatalf("expected Added rules section, got:\n%s", out)
	}
	if !strings.Contains(out, "github.delete_*") {
		t.Fatalf("expected added rule tool in output, got:\n%s", out)
	}
}

func TestFormatDiffOutput_Impact(t *testing.T) {
	diff := &toolpolicy.DiffResult{
		Added:   []toolpolicy.ToolRule{},
		Removed: []toolpolicy.ToolRule{},
		Changed: []toolpolicy.RuleChange{},
		Impact: []toolpolicy.ImpactedAction{
			{Tool: "github.push", Protocol: "git", OldDecision: "review", NewDecision: "block"},
		},
	}

	out := formatDiffOutput(diff)
	if !strings.Contains(out, "Impacted actions:") {
		t.Fatalf("expected Impacted actions section, got:\n%s", out)
	}
	if !strings.Contains(out, "review -> block") {
		t.Fatalf("expected impact detail in output, got:\n%s", out)
	}
}

func TestFormatDiffOutput_NoDifferences(t *testing.T) {
	diff := &toolpolicy.DiffResult{
		Added:   []toolpolicy.ToolRule{},
		Removed: []toolpolicy.ToolRule{},
		Changed: []toolpolicy.RuleChange{},
		Impact:  []toolpolicy.ImpactedAction{},
	}

	out := formatDiffOutput(diff)
	if !strings.Contains(out, "No differences found") {
		t.Fatalf("expected no differences message, got:\n%s", out)
	}
}
