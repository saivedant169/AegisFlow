package toolpolicy

import (
	"path"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// ToolRule defines a single policy rule for tool access.
type ToolRule struct {
	Protocol   string `yaml:"protocol" json:"protocol"`     // "mcp", "http", "shell", "sql", "git", "*"
	Tool       string `yaml:"tool" json:"tool"`             // glob pattern, e.g. "github.list_*"
	Target     string `yaml:"target" json:"target"`         // glob pattern for target, optional
	Capability string `yaml:"capability" json:"capability"` // "read", "write", "delete", etc. Optional.
	Decision   string `yaml:"decision" json:"decision"`     // "allow", "review", "block"
}

// Engine evaluates ActionEnvelopes against a list of ToolRules.
type Engine struct {
	rules           []ToolRule
	defaultDecision envelope.Decision
}

// NewEngine creates a new tool policy engine.
// defaultDecision is used when no rule matches ("allow", "review", or "block").
func NewEngine(rules []ToolRule, defaultDecision string) *Engine {
	dd := envelope.Decision(defaultDecision)
	switch dd {
	case envelope.DecisionAllow, envelope.DecisionReview, envelope.DecisionBlock:
		// valid
	default:
		dd = envelope.DecisionBlock
	}
	return &Engine{
		rules:           rules,
		defaultDecision: dd,
	}
}

// Evaluate checks the envelope against rules. First matching rule wins.
func (e *Engine) Evaluate(env *envelope.ActionEnvelope) envelope.Decision {
	for _, rule := range e.rules {
		if e.matches(rule, env) {
			return envelope.Decision(rule.Decision)
		}
	}
	return e.defaultDecision
}

func (e *Engine) matches(rule ToolRule, env *envelope.ActionEnvelope) bool {
	// Protocol match
	if rule.Protocol != "" && rule.Protocol != "*" {
		if rule.Protocol != string(env.Protocol) {
			return false
		}
	}

	// Tool match (glob)
	if rule.Tool != "" && rule.Tool != "*" {
		matched, _ := path.Match(rule.Tool, env.Tool)
		if !matched {
			return false
		}
	}

	// Target match (glob, optional)
	if rule.Target != "" && rule.Target != "*" {
		matched, _ := path.Match(rule.Target, env.Target)
		if !matched {
			return false
		}
	}

	// Capability match (optional)
	if rule.Capability != "" && rule.Capability != "*" {
		if rule.Capability != string(env.RequestedCapability) {
			return false
		}
	}

	return true
}
