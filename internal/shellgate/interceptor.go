package shellgate

import (
	"fmt"
	"strings"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// Result holds the outcome of a shell command evaluation.
type Result struct {
	Decision   envelope.Decision `json:"decision"`
	EnvelopeID string            `json:"envelope_id"`
	Message    string            `json:"message"`
}

// Interceptor evaluates shell commands against tool policies and records
// them in the evidence chain before execution.
type Interceptor struct {
	policy         *toolpolicy.Engine
	evidence       *evidence.SessionChain
	approvals      *approval.Queue
	blockDangerous bool
}

// NewInterceptor creates a new shell command interceptor.
func NewInterceptor(pe *toolpolicy.Engine, ev *evidence.SessionChain, aq *approval.Queue, blockDangerous bool) *Interceptor {
	return &Interceptor{
		policy:         pe,
		evidence:       ev,
		approvals:      aq,
		blockDangerous: blockDangerous,
	}
}

// Evaluate inspects a shell command, runs it through policy evaluation,
// records it in the evidence chain, and returns the decision.
func (i *Interceptor) Evaluate(cmd string, args []string, workDir string) (*Result, error) {
	// Check for dangerous commands first.
	if i.blockDangerous && IsDangerous(cmd, args) {
		env := buildEnvelope(cmd, args, workDir)
		env.PolicyDecision = envelope.DecisionBlock
		if i.evidence != nil {
			i.evidence.Record(env)
		}
		return &Result{
			Decision:   envelope.DecisionBlock,
			EnvelopeID: env.ID,
			Message:    fmt.Sprintf("blocked dangerous command: %s %s", cmd, strings.Join(args, " ")),
		}, nil
	}

	env := buildEnvelope(cmd, args, workDir)

	// Evaluate against tool policy engine.
	decision := i.policy.Evaluate(env)
	env.PolicyDecision = decision

	// Record in evidence chain.
	if i.evidence != nil {
		i.evidence.Record(env)
	}

	// If review is required, submit to approval queue.
	if decision == envelope.DecisionReview && i.approvals != nil {
		i.approvals.Submit(env)
	}

	msg := fmt.Sprintf("%s: %s %s", decision, cmd, strings.Join(args, " "))

	return &Result{
		Decision:   decision,
		EnvelopeID: env.ID,
		Message:    msg,
	}, nil
}

// buildEnvelope constructs an ActionEnvelope for the given shell command.
func buildEnvelope(cmd string, args []string, workDir string) *envelope.ActionEnvelope {
	capability := inferShellCapability(cmd, args)
	toolName := "shell." + baseCommand(cmd)

	env := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "shell-agent"},
		"shell-session",
		envelope.ProtocolShell,
		toolName,
		workDir,
		capability,
	)
	env.Parameters["cmd"] = cmd
	env.Parameters["args"] = args
	return env
}

// inferShellCapability maps a shell command to a capability.
func inferShellCapability(cmd string, args []string) envelope.Capability {
	base := baseCommand(cmd)
	fullArgs := strings.Join(args, " ")

	// Delete commands
	switch base {
	case "rm", "kill", "pkill":
		return envelope.CapDelete
	}

	// Deploy commands
	switch base {
	case "terraform":
		if strings.Contains(fullArgs, "apply") || strings.Contains(fullArgs, "destroy") {
			return envelope.CapDeploy
		}
	case "kubectl":
		if strings.Contains(fullArgs, "apply") || strings.Contains(fullArgs, "delete") {
			return envelope.CapDeploy
		}
	case "git":
		if strings.Contains(fullArgs, "push") {
			return envelope.CapDeploy
		}
	}

	return envelope.CapExecute
}
