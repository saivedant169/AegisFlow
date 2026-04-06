package githubgate

import (
	"fmt"

	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// Result is the outcome of evaluating a GitHub operation.
type Result struct {
	Decision   envelope.Decision `json:"decision"`
	EnvelopeID string            `json:"envelope_id"`
	Message    string            `json:"message"`
	RiskLevel  RiskLevel         `json:"risk_level"`
}

// Interceptor evaluates GitHub API operations against tool policies and
// records every evaluation in the evidence chain.
type Interceptor struct {
	engine *toolpolicy.Engine
	chain  *evidence.SessionChain
	actor  envelope.ActorInfo
}

// NewInterceptor creates a GitHub operation interceptor.
func NewInterceptor(engine *toolpolicy.Engine, chain *evidence.SessionChain, actor envelope.ActorInfo) *Interceptor {
	return &Interceptor{
		engine: engine,
		chain:  chain,
		actor:  actor,
	}
}

// Evaluate checks a GitHub operation against tool policies, records the
// result in the evidence chain, and returns the decision.
func (i *Interceptor) Evaluate(operation string, repo string, params map[string]any) (*Result, error) {
	toolName := "github." + operation
	capability, risk := ClassifyOperation(operation)

	env := envelope.NewEnvelope(
		i.actor,
		fmt.Sprintf("github:%s", operation),
		envelope.ProtocolGit,
		toolName,
		repo,
		capability,
	)

	// Copy caller-supplied parameters into the envelope.
	for k, v := range params {
		env.Parameters[k] = v
	}

	// Evaluate against tool policy rules.
	decision := i.engine.Evaluate(env)
	env.PolicyDecision = decision

	// Record in the evidence chain.
	_, err := i.chain.Record(env)
	if err != nil {
		return nil, fmt.Errorf("recording evidence: %w", err)
	}

	msg := fmt.Sprintf("%s %s on %s: %s (risk=%s)", decision, toolName, repo, operation, risk)

	return &Result{
		Decision:   decision,
		EnvelopeID: env.ID,
		Message:    msg,
		RiskLevel:  risk,
	}, nil
}
