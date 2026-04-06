package toolpolicy

import (
	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// SimulateResult shows what would happen under a given policy.
type SimulateResult struct {
	Action   string               `json:"action"`
	Decision string               `json:"decision"`
	Trace    *PolicyDecisionTrace `json:"trace"`
}

// DiffResult shows what changes between two policy sets.
type DiffResult struct {
	Added   []ToolRule       `json:"added"`
	Removed []ToolRule       `json:"removed"`
	Changed []RuleChange     `json:"changed"`
	Impact  []ImpactedAction `json:"impact"` // actions that would change decision
}

// RuleChange describes a rule that exists at the same index but differs.
type RuleChange struct {
	Index  int      `json:"index"`
	Before ToolRule `json:"before"`
	After  ToolRule `json:"after"`
}

// ImpactedAction describes an action whose decision would change.
type ImpactedAction struct {
	Tool        string `json:"tool"`
	Protocol    string `json:"protocol"`
	OldDecision string `json:"old_decision"`
	NewDecision string `json:"new_decision"`
}

// Simulate evaluates an envelope against the engine and returns a full trace.
func (e *Engine) Simulate(env *envelope.ActionEnvelope) *SimulateResult {
	trace := e.EvaluateWithTrace(env)
	return &SimulateResult{
		Action:   env.Tool,
		Decision: trace.Decision,
		Trace:    trace,
	}
}

// DiffPolicies compares two rule sets and reports structural differences and
// decision impact on the provided test actions.
func DiffPolicies(oldRules, newRules []ToolRule, testActions []*envelope.ActionEnvelope) *DiffResult {
	result := &DiffResult{
		Added:   []ToolRule{},
		Removed: []ToolRule{},
		Changed: []RuleChange{},
		Impact:  []ImpactedAction{},
	}

	minLen := len(oldRules)
	if len(newRules) < minLen {
		minLen = len(newRules)
	}

	// Compare rules at overlapping indices.
	for i := 0; i < minLen; i++ {
		if !rulesEqual(oldRules[i], newRules[i]) {
			result.Changed = append(result.Changed, RuleChange{
				Index:  i,
				Before: oldRules[i],
				After:  newRules[i],
			})
		}
	}

	// Rules only in newRules (added).
	for i := minLen; i < len(newRules); i++ {
		result.Added = append(result.Added, newRules[i])
	}

	// Rules only in oldRules (removed).
	for i := minLen; i < len(oldRules); i++ {
		result.Removed = append(result.Removed, oldRules[i])
	}

	// Evaluate impact on test actions.
	oldEngine := NewEngine(oldRules, "block")
	newEngine := NewEngine(newRules, "block")

	for _, env := range testActions {
		oldDec := oldEngine.Evaluate(env)
		newDec := newEngine.Evaluate(env)
		if oldDec != newDec {
			result.Impact = append(result.Impact, ImpactedAction{
				Tool:        env.Tool,
				Protocol:    string(env.Protocol),
				OldDecision: string(oldDec),
				NewDecision: string(newDec),
			})
		}
	}

	return result
}

func rulesEqual(a, b ToolRule) bool {
	return a.Protocol == b.Protocol &&
		a.Tool == b.Tool &&
		a.Target == b.Target &&
		a.Capability == b.Capability &&
		a.Decision == b.Decision
}
