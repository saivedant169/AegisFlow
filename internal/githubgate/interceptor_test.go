package githubgate

import (
	"testing"

	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

func newTestInterceptor(rules []toolpolicy.ToolRule, defaultDecision string) *Interceptor {
	engine := toolpolicy.NewEngine(rules, defaultDecision)
	chain := evidence.NewSessionChain("test-session")
	actor := envelope.ActorInfo{
		Type:      "agent",
		ID:        "test-agent",
		SessionID: "test-session",
		TenantID:  "test-tenant",
	}
	return NewInterceptor(engine, chain, actor)
}

func TestAllowListRepos(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "git", Tool: "github.list_*", Decision: "allow"},
	}
	ic := newTestInterceptor(rules, "block")

	res, err := ic.Evaluate("list_repos", "org/repo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != envelope.DecisionAllow {
		t.Errorf("decision = %q, want %q", res.Decision, envelope.DecisionAllow)
	}
}

func TestAllowGetPullRequest(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "git", Tool: "github.get_*", Decision: "allow"},
	}
	ic := newTestInterceptor(rules, "block")

	res, err := ic.Evaluate("get_pull_request", "org/repo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != envelope.DecisionAllow {
		t.Errorf("decision = %q, want %q", res.Decision, envelope.DecisionAllow)
	}
}

func TestReviewCreatePullRequest(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "git", Tool: "github.create_pull_request", Decision: "review"},
	}
	ic := newTestInterceptor(rules, "block")

	res, err := ic.Evaluate("create_pull_request", "org/repo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != envelope.DecisionReview {
		t.Errorf("decision = %q, want %q", res.Decision, envelope.DecisionReview)
	}
}

func TestBlockDeleteRepo(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "git", Tool: "github.delete_*", Decision: "block"},
	}
	ic := newTestInterceptor(rules, "review")

	res, err := ic.Evaluate("delete_repo", "org/repo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != envelope.DecisionBlock {
		t.Errorf("decision = %q, want %q", res.Decision, envelope.DecisionBlock)
	}
	if res.RiskLevel != RiskCritical {
		t.Errorf("risk = %q, want %q", res.RiskLevel, RiskCritical)
	}
}

func TestBlockMergePullRequest(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "git", Tool: "github.merge_pull_request", Decision: "block"},
	}
	ic := newTestInterceptor(rules, "allow")

	res, err := ic.Evaluate("merge_pull_request", "org/repo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Decision != envelope.DecisionBlock {
		t.Errorf("decision = %q, want %q", res.Decision, envelope.DecisionBlock)
	}
}

func TestEvidenceRecorded(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "git", Tool: "*", Decision: "allow"},
	}
	engine := toolpolicy.NewEngine(rules, "block")
	chain := evidence.NewSessionChain("evidence-test")
	actor := envelope.ActorInfo{
		Type:      "agent",
		ID:        "test-agent",
		SessionID: "evidence-test",
		TenantID:  "test-tenant",
	}
	ic := NewInterceptor(engine, chain, actor)

	_, err := ic.Evaluate("list_repos", "org/repo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = ic.Evaluate("create_issue", "org/repo", map[string]any{"title": "bug"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if chain.Count() != 2 {
		t.Errorf("evidence count = %d, want 2", chain.Count())
	}

	records := chain.Records()
	if records[0].Envelope.Tool != "github.list_repos" {
		t.Errorf("record[0].Tool = %q, want %q", records[0].Envelope.Tool, "github.list_repos")
	}
	if records[1].Envelope.Tool != "github.create_issue" {
		t.Errorf("record[1].Tool = %q, want %q", records[1].Envelope.Tool, "github.create_issue")
	}
}

func TestRepoInTarget(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "git", Tool: "*", Decision: "allow"},
	}
	engine := toolpolicy.NewEngine(rules, "block")
	chain := evidence.NewSessionChain("target-test")
	actor := envelope.ActorInfo{
		Type:      "agent",
		ID:        "test-agent",
		SessionID: "target-test",
		TenantID:  "test-tenant",
	}
	ic := NewInterceptor(engine, chain, actor)

	_, err := ic.Evaluate("get_repo", "myorg/myrepo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records := chain.Records()
	if records[0].Envelope.Target != "myorg/myrepo" {
		t.Errorf("target = %q, want %q", records[0].Envelope.Target, "myorg/myrepo")
	}
}

func TestParamsPassedToEnvelope(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "git", Tool: "*", Decision: "allow"},
	}
	engine := toolpolicy.NewEngine(rules, "block")
	chain := evidence.NewSessionChain("params-test")
	actor := envelope.ActorInfo{
		Type:      "agent",
		ID:        "test-agent",
		SessionID: "params-test",
		TenantID:  "test-tenant",
	}
	ic := NewInterceptor(engine, chain, actor)

	params := map[string]any{
		"title": "Fix bug",
		"draft": true,
		"base":  "main",
	}
	_, err := ic.Evaluate("create_pull_request", "org/repo", params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	records := chain.Records()
	env := records[0].Envelope
	if env.Parameters["title"] != "Fix bug" {
		t.Errorf("params[title] = %v, want %q", env.Parameters["title"], "Fix bug")
	}
	if env.Parameters["draft"] != true {
		t.Errorf("params[draft] = %v, want true", env.Parameters["draft"])
	}
	if env.Parameters["base"] != "main" {
		t.Errorf("params[base] = %v, want %q", env.Parameters["base"], "main")
	}
}
