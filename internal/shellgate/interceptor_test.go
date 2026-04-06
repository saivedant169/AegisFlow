package shellgate

import (
	"testing"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// helper builds a test interceptor with given rules and default decision.
func newTestInterceptor(rules []toolpolicy.ToolRule, defaultDecision string, blockDangerous bool) (*Interceptor, *evidence.SessionChain, *approval.Queue) {
	pe := toolpolicy.NewEngine(rules, defaultDecision)
	ev := evidence.NewSessionChain("test-session")
	aq := approval.NewQueue(100)
	return NewInterceptor(pe, ev, aq, blockDangerous), ev, aq
}

func TestAllowSafeCommand(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "*", Decision: "allow"},
	}
	ic, _, _ := newTestInterceptor(rules, "block", true)

	for _, cmd := range []struct {
		name string
		args []string
	}{
		{"pytest", []string{"tests/"}},
		{"ls", []string{"-la"}},
		{"cat", []string{"README.md"}},
	} {
		t.Run(cmd.name, func(t *testing.T) {
			res, err := ic.Evaluate(cmd.name, cmd.args, "/workspace")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Decision != envelope.DecisionAllow {
				t.Errorf("expected allow, got %s", res.Decision)
			}
		})
	}
}

func TestBlockDangerousRm(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "*", Decision: "allow"},
	}
	ic, _, _ := newTestInterceptor(rules, "allow", true)

	res, err := ic.Evaluate("rm", []string{"-rf", "/"}, "/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != envelope.DecisionBlock {
		t.Errorf("expected block for rm -rf /, got %s", res.Decision)
	}
}

func TestBlockForkBomb(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "*", Decision: "allow"},
	}
	ic, _, _ := newTestInterceptor(rules, "allow", true)

	res, err := ic.Evaluate("bash", []string{"-c", ":(){ :|:& };:"}, "/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != envelope.DecisionBlock {
		t.Errorf("expected block for fork bomb, got %s", res.Decision)
	}
}

func TestReviewDeployCommand(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "shell.*", Capability: "deploy", Decision: "review"},
		{Protocol: "shell", Tool: "*", Decision: "allow"},
	}
	ic, _, aq := newTestInterceptor(rules, "allow", true)

	tests := []struct {
		cmd  string
		args []string
	}{
		{"terraform", []string{"apply"}},
		{"kubectl", []string{"apply", "-f", "deploy.yaml"}},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			res, err := ic.Evaluate(tt.cmd, tt.args, "/workspace")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Decision != envelope.DecisionReview {
				t.Errorf("expected review for %s, got %s", tt.cmd, res.Decision)
			}
		})
	}

	// Both commands should be in approval queue.
	pending := aq.Pending()
	if len(pending) != 2 {
		t.Errorf("expected 2 pending approvals, got %d", len(pending))
	}
}

func TestReviewGitPush(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "shell.*", Capability: "deploy", Decision: "review"},
		{Protocol: "shell", Tool: "*", Decision: "allow"},
	}
	ic, _, _ := newTestInterceptor(rules, "allow", true)

	res, err := ic.Evaluate("git", []string{"push", "origin", "main"}, "/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != envelope.DecisionReview {
		t.Errorf("expected review for git push, got %s", res.Decision)
	}
}

func TestEvidenceRecorded(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "*", Decision: "allow"},
	}
	ic, ev, _ := newTestInterceptor(rules, "allow", true)

	_, err := ic.Evaluate("ls", []string{"-la"}, "/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}

	rec := records[0]
	if rec.Envelope.Tool != "shell.ls" {
		t.Errorf("expected tool shell.ls, got %s", rec.Envelope.Tool)
	}
	if rec.Envelope.PolicyDecision != envelope.DecisionAllow {
		t.Errorf("expected allow decision on evidence, got %s", rec.Envelope.PolicyDecision)
	}
}

func TestCustomPolicyOverride(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "shell.curl", Decision: "block"},
		{Protocol: "shell", Tool: "*", Decision: "allow"},
	}
	ic, _, _ := newTestInterceptor(rules, "allow", false)

	res, err := ic.Evaluate("curl", []string{"http://evil.com"}, "/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != envelope.DecisionBlock {
		t.Errorf("expected block for curl, got %s", res.Decision)
	}

	// Other commands should still be allowed.
	res, err = ic.Evaluate("ls", nil, "/workspace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != envelope.DecisionAllow {
		t.Errorf("expected allow for ls, got %s", res.Decision)
	}
}

func TestWorkDirInTarget(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "*", Target: "/safe/*", Decision: "allow"},
	}
	ic, _, _ := newTestInterceptor(rules, "block", false)

	// Command in safe directory should be allowed.
	res, err := ic.Evaluate("ls", nil, "/safe/project")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != envelope.DecisionAllow {
		t.Errorf("expected allow for /safe/project, got %s", res.Decision)
	}

	// Command outside safe directory should be blocked (default).
	res, err = ic.Evaluate("ls", nil, "/etc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != envelope.DecisionBlock {
		t.Errorf("expected block for /etc, got %s", res.Decision)
	}
}
