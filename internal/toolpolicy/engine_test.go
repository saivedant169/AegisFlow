package toolpolicy

import (
	"testing"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func testEnvelope(protocol envelope.Protocol, tool, target string, cap envelope.Capability) *envelope.ActionEnvelope {
	return &envelope.ActionEnvelope{
		ID:                  "test-1",
		Protocol:            protocol,
		Tool:                tool,
		Target:              target,
		RequestedCapability: cap,
		Actor:               envelope.ActorInfo{Type: "agent", ID: "agent-1", TenantID: "tenant-1"},
		Task:                "test-task",
	}
}

func TestAllowRule(t *testing.T) {
	engine := NewEngine([]ToolRule{
		{Protocol: "git", Tool: "github.list_pull_requests", Decision: "allow"},
	}, "block")

	env := testEnvelope(envelope.ProtocolGit, "github.list_pull_requests", "repo/main", envelope.CapRead)
	decision := engine.Evaluate(env)
	if decision != envelope.DecisionAllow {
		t.Fatalf("expected allow, got %s", decision)
	}
}

func TestBlockRule(t *testing.T) {
	engine := NewEngine([]ToolRule{
		{Protocol: "git", Tool: "github.merge_pull_request", Decision: "block"},
	}, "allow")

	env := testEnvelope(envelope.ProtocolGit, "github.merge_pull_request", "repo/main", envelope.CapWrite)
	decision := engine.Evaluate(env)
	if decision != envelope.DecisionBlock {
		t.Fatalf("expected block, got %s", decision)
	}
}

func TestReviewRule(t *testing.T) {
	engine := NewEngine([]ToolRule{
		{Protocol: "git", Tool: "github.create_pull_request", Decision: "review"},
	}, "block")

	env := testEnvelope(envelope.ProtocolGit, "github.create_pull_request", "repo/main", envelope.CapWrite)
	decision := engine.Evaluate(env)
	if decision != envelope.DecisionReview {
		t.Fatalf("expected review, got %s", decision)
	}
}

func TestGlobMatching(t *testing.T) {
	engine := NewEngine([]ToolRule{
		{Protocol: "git", Tool: "github.list_*", Decision: "allow"},
		{Protocol: "git", Tool: "github.*", Decision: "review"},
	}, "block")

	// Should match first rule (more specific glob)
	env1 := testEnvelope(envelope.ProtocolGit, "github.list_issues", "repo", envelope.CapRead)
	if d := engine.Evaluate(env1); d != envelope.DecisionAllow {
		t.Fatalf("expected allow for list_issues, got %s", d)
	}

	// Should match second rule
	env2 := testEnvelope(envelope.ProtocolGit, "github.create_branch", "repo", envelope.CapWrite)
	if d := engine.Evaluate(env2); d != envelope.DecisionReview {
		t.Fatalf("expected review for create_branch, got %s", d)
	}
}

func TestCapabilityFilter(t *testing.T) {
	engine := NewEngine([]ToolRule{
		{Protocol: "sql", Tool: "*", Capability: "read", Decision: "allow"},
		{Protocol: "sql", Tool: "*", Capability: "write", Decision: "review"},
		{Protocol: "sql", Tool: "*", Capability: "delete", Decision: "block"},
	}, "block")

	envRead := testEnvelope(envelope.ProtocolSQL, "postgres.query", "db/users", envelope.CapRead)
	if d := engine.Evaluate(envRead); d != envelope.DecisionAllow {
		t.Fatalf("expected allow for read, got %s", d)
	}

	envWrite := testEnvelope(envelope.ProtocolSQL, "postgres.query", "db/users", envelope.CapWrite)
	if d := engine.Evaluate(envWrite); d != envelope.DecisionReview {
		t.Fatalf("expected review for write, got %s", d)
	}

	envDelete := testEnvelope(envelope.ProtocolSQL, "postgres.query", "db/users", envelope.CapDelete)
	if d := engine.Evaluate(envDelete); d != envelope.DecisionBlock {
		t.Fatalf("expected block for delete, got %s", d)
	}
}

func TestDefaultDecisionWhenNoMatch(t *testing.T) {
	engine := NewEngine([]ToolRule{
		{Protocol: "git", Tool: "github.list_*", Decision: "allow"},
	}, "block")

	// No rule matches shell protocol
	env := testEnvelope(envelope.ProtocolShell, "shell.exec", "/bin/bash", envelope.CapExecute)
	if d := engine.Evaluate(env); d != envelope.DecisionBlock {
		t.Fatalf("expected default block, got %s", d)
	}
}

func TestProtocolWildcard(t *testing.T) {
	engine := NewEngine([]ToolRule{
		{Protocol: "*", Tool: "*", Capability: "delete", Decision: "block"},
	}, "allow")

	env := testEnvelope(envelope.ProtocolHTTP, "api.delete_user", "service/users", envelope.CapDelete)
	if d := engine.Evaluate(env); d != envelope.DecisionBlock {
		t.Fatalf("expected block for delete on any protocol, got %s", d)
	}
}

func TestTargetFilter(t *testing.T) {
	engine := NewEngine([]ToolRule{
		{Protocol: "shell", Tool: "shell.exec", Target: "/usr/bin/*", Decision: "allow"},
		{Protocol: "shell", Tool: "shell.exec", Decision: "block"},
	}, "block")

	envSafe := testEnvelope(envelope.ProtocolShell, "shell.exec", "/usr/bin/pytest", envelope.CapExecute)
	if d := engine.Evaluate(envSafe); d != envelope.DecisionAllow {
		t.Fatalf("expected allow for safe path, got %s", d)
	}

	envDanger := testEnvelope(envelope.ProtocolShell, "shell.exec", "/bin/rm", envelope.CapExecute)
	if d := engine.Evaluate(envDanger); d != envelope.DecisionBlock {
		t.Fatalf("expected block for dangerous path, got %s", d)
	}
}

func TestEmptyRulesUsesDefault(t *testing.T) {
	engine := NewEngine(nil, "allow")
	env := testEnvelope(envelope.ProtocolMCP, "some.tool", "target", envelope.CapRead)
	if d := engine.Evaluate(env); d != envelope.DecisionAllow {
		t.Fatalf("expected default allow, got %s", d)
	}
}
