package toolpolicy

import (
	"path"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// PolicyDecisionTrace explains why a decision was made.
type PolicyDecisionTrace struct {
	Decision     string          `json:"decision"`
	MatchedRule  *ToolRule       `json:"matched_rule,omitempty"`
	MatchedIndex int             `json:"matched_index"` // -1 if default
	RulesChecked int             `json:"rules_checked"`
	DefaultUsed  bool            `json:"default_used"`
	CheckTrace   []RuleCheckStep `json:"check_trace"`
}

// RuleCheckStep records the outcome of checking a single rule.
type RuleCheckStep struct {
	RuleIndex  int      `json:"rule_index"`
	Rule       ToolRule `json:"rule"`
	Matched    bool     `json:"matched"`
	FailReason string   `json:"fail_reason,omitempty"` // "protocol mismatch", "tool glob no match", etc.
}

// EvaluateWithTrace checks the envelope against rules and returns the full
// decision trace so operators can see the decision path.
func (e *Engine) EvaluateWithTrace(env *envelope.ActionEnvelope) *PolicyDecisionTrace {
	e.mu.RLock()
	defer e.mu.RUnlock()

	trace := &PolicyDecisionTrace{
		MatchedIndex: -1,
		CheckTrace:   make([]RuleCheckStep, 0, len(e.rules)),
	}

	for i, rule := range e.rules {
		step := RuleCheckStep{
			RuleIndex: i,
			Rule:      rule,
		}

		reason := matchReason(rule, env)
		if reason == "" {
			step.Matched = true
			trace.CheckTrace = append(trace.CheckTrace, step)
			trace.RulesChecked = i + 1
			trace.Decision = rule.Decision
			trace.MatchedRule = &e.rules[i]
			trace.MatchedIndex = i
			return trace
		}

		step.FailReason = reason
		trace.CheckTrace = append(trace.CheckTrace, step)
	}

	// No rule matched — use default.
	trace.RulesChecked = len(e.rules)
	trace.DefaultUsed = true
	trace.Decision = string(e.defaultDecision)
	return trace
}

// matchReason returns "" if the rule matches, or a human-readable reason it
// did not match.
func matchReason(rule ToolRule, env *envelope.ActionEnvelope) string {
	// Protocol
	if rule.Protocol != "" && rule.Protocol != "*" {
		if rule.Protocol != string(env.Protocol) {
			return "protocol mismatch"
		}
	}

	// Tool (glob)
	if rule.Tool != "" && rule.Tool != "*" {
		matched, _ := path.Match(rule.Tool, env.Tool)
		if !matched {
			return "tool glob no match"
		}
	}

	// Target (glob, optional)
	if rule.Target != "" && rule.Target != "*" {
		matched, _ := path.Match(rule.Target, env.Target)
		if !matched {
			return "target glob no match"
		}
	}

	// Capability
	if rule.Capability != "" && rule.Capability != "*" {
		if rule.Capability != string(env.RequestedCapability) {
			return "capability mismatch"
		}
	}

	return ""
}
