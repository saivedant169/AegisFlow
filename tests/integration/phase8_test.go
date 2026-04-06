package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/githubgate"
	"github.com/saivedant169/AegisFlow/internal/httpgate"
	"github.com/saivedant169/AegisFlow/internal/shellgate"
	"github.com/saivedant169/AegisFlow/internal/sqlgate"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// ---------- Phase 8: Protocol Connector E2E Tests ----------

// ===== Shell Interceptor =====

func TestShellInterceptorAllowE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "shell.ls", Decision: "allow"},
	}, "block")
	ev := evidence.NewSessionChain("sess-shell-allow")
	aq := approval.NewQueue(100)

	interceptor := shellgate.NewInterceptor(pe, ev, aq, true)

	result, err := interceptor.Evaluate("ls", []string{"-la", "/tmp"}, "/home/user")
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.Decision != envelope.DecisionAllow {
		t.Errorf("expected allow, got %s", result.Decision)
	}
	if result.EnvelopeID == "" {
		t.Error("expected non-empty envelope ID")
	}

	// Evidence should have 1 record.
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionAllow {
		t.Errorf("expected allow in evidence, got %s", records[0].Envelope.PolicyDecision)
	}
	if records[0].Envelope.Tool != "shell.ls" {
		t.Errorf("expected tool 'shell.ls', got %s", records[0].Envelope.Tool)
	}
}

func TestShellInterceptorBlockDangerousE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "shell.*", Decision: "allow"},
	}, "allow")
	ev := evidence.NewSessionChain("sess-shell-dangerous")
	aq := approval.NewQueue(100)

	// blockDangerous=true overrides policy for dangerous commands.
	interceptor := shellgate.NewInterceptor(pe, ev, aq, true)

	result, err := interceptor.Evaluate("rm", []string{"-rf", "/"}, "/")
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.Decision != envelope.DecisionBlock {
		t.Fatalf("expected block for 'rm -rf /', got %s", result.Decision)
	}
	if !strings.Contains(result.Message, "dangerous") {
		t.Errorf("expected 'dangerous' in message, got %q", result.Message)
	}

	// Evidence should record the block.
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionBlock {
		t.Errorf("expected block in evidence, got %s", records[0].Envelope.PolicyDecision)
	}
}

func TestShellInterceptorReviewDeployE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "shell", Tool: "shell.terraform", Decision: "review"},
	}, "block")
	ev := evidence.NewSessionChain("sess-shell-review")
	aq := approval.NewQueue(100)

	interceptor := shellgate.NewInterceptor(pe, ev, aq, true)

	result, err := interceptor.Evaluate("terraform", []string{"apply", "-auto-approve"}, "/infra")
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.Decision != envelope.DecisionReview {
		t.Errorf("expected review for 'terraform apply', got %s", result.Decision)
	}

	// Should be in the approval queue.
	pending := aq.Pending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending approval, got %d", len(pending))
	}
	if pending[0].Envelope.Tool != "shell.terraform" {
		t.Errorf("expected tool 'shell.terraform' in queue, got %s", pending[0].Envelope.Tool)
	}

	// Evidence should record review.
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionReview {
		t.Errorf("expected review in evidence, got %s", records[0].Envelope.PolicyDecision)
	}
}

// ===== SQL Interceptor =====

func TestSQLInterceptorAllowSelectE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "sql", Tool: "sql.select", Decision: "allow"},
	}, "block")
	ev := evidence.NewSessionChain("sess-sql-allow")

	interceptor := sqlgate.NewInterceptor(pe, ev, true)

	result, err := interceptor.Evaluate("SELECT * FROM users WHERE id = 1", "production")
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.Decision != envelope.DecisionAllow {
		t.Errorf("expected allow for SELECT, got %s", result.Decision)
	}
	if result.Operation != "select" {
		t.Errorf("expected operation 'select', got %q", result.Operation)
	}

	// Evidence should have the record.
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.RequestedCapability != envelope.CapRead {
		t.Errorf("expected read capability, got %s", records[0].Envelope.RequestedCapability)
	}
}

func TestSQLInterceptorBlockDropE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "sql", Tool: "sql.select", Decision: "allow"},
	}, "block")
	ev := evidence.NewSessionChain("sess-sql-drop")

	// blockDangerous=true should auto-block DROP TABLE.
	interceptor := sqlgate.NewInterceptor(pe, ev, true)

	result, err := interceptor.Evaluate("DROP TABLE users", "production")
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.Decision != envelope.DecisionBlock {
		t.Fatalf("expected block for DROP TABLE, got %s", result.Decision)
	}
	if result.Operation != "drop_table" {
		t.Errorf("expected operation 'drop_table', got %q", result.Operation)
	}
	if !strings.Contains(result.Message, "dangerous") {
		t.Errorf("expected 'dangerous' in message, got %q", result.Message)
	}

	// Evidence should record block.
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionBlock {
		t.Errorf("expected block in evidence, got %s", records[0].Envelope.PolicyDecision)
	}
}

func TestSQLInterceptorBlockDeleteWithoutWhereE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "sql", Tool: "sql.*", Decision: "allow"},
	}, "allow")
	ev := evidence.NewSessionChain("sess-sql-nowhere")

	// blockDangerous=true should block DELETE without WHERE.
	interceptor := sqlgate.NewInterceptor(pe, ev, true)

	result, err := interceptor.Evaluate("DELETE FROM users", "production")
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.Decision != envelope.DecisionBlock {
		t.Fatalf("expected block for DELETE without WHERE, got %s", result.Decision)
	}
	if result.Operation != "delete" {
		t.Errorf("expected operation 'delete', got %q", result.Operation)
	}

	// Evidence should record the block.
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
}

// ===== GitHub Interceptor =====

func TestGitHubInterceptorAllowReadE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "git", Tool: "github.list_repos", Decision: "allow"},
	}, "block")
	ev := evidence.NewSessionChain("sess-gh-allow")
	actor := envelope.ActorInfo{Type: "agent", ID: "test-agent"}

	interceptor := githubgate.NewInterceptor(pe, ev, actor)

	result, err := interceptor.Evaluate("list_repos", "myorg/myrepo", map[string]any{"org": "myorg"})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.Decision != envelope.DecisionAllow {
		t.Errorf("expected allow for list_repos, got %s", result.Decision)
	}
	if result.RiskLevel != githubgate.RiskLow {
		t.Errorf("expected risk low, got %s", result.RiskLevel)
	}

	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.Tool != "github.list_repos" {
		t.Errorf("expected tool 'github.list_repos', got %s", records[0].Envelope.Tool)
	}
	if records[0].Envelope.RequestedCapability != envelope.CapRead {
		t.Errorf("expected read capability, got %s", records[0].Envelope.RequestedCapability)
	}
}

func TestGitHubInterceptorBlockDeleteE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "git", Tool: "github.delete_*", Decision: "block"},
	}, "block")
	ev := evidence.NewSessionChain("sess-gh-block")
	actor := envelope.ActorInfo{Type: "agent", ID: "test-agent"}

	interceptor := githubgate.NewInterceptor(pe, ev, actor)

	result, err := interceptor.Evaluate("delete_repo", "myorg/critical-repo", map[string]any{"confirm": true})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.Decision != envelope.DecisionBlock {
		t.Fatalf("expected block for delete_repo, got %s", result.Decision)
	}
	if result.RiskLevel != githubgate.RiskCritical {
		t.Errorf("expected risk critical, got %s", result.RiskLevel)
	}

	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionBlock {
		t.Errorf("expected block in evidence, got %s", records[0].Envelope.PolicyDecision)
	}
}

func TestGitHubInterceptorReviewPRE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "git", Tool: "github.create_pull_request", Decision: "review"},
	}, "block")
	ev := evidence.NewSessionChain("sess-gh-review")
	actor := envelope.ActorInfo{Type: "agent", ID: "test-agent"}

	interceptor := githubgate.NewInterceptor(pe, ev, actor)

	result, err := interceptor.Evaluate("create_pull_request", "myorg/myrepo", map[string]any{
		"title": "feat: add new endpoint",
		"base":  "main",
		"head":  "feature/new-api",
	})
	if err != nil {
		t.Fatalf("Evaluate failed: %v", err)
	}

	if result.Decision != envelope.DecisionReview {
		t.Errorf("expected review for create_pull_request, got %s", result.Decision)
	}
	if result.RiskLevel != githubgate.RiskMedium {
		t.Errorf("expected risk medium, got %s", result.RiskLevel)
	}

	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionReview {
		t.Errorf("expected review in evidence, got %s", records[0].Envelope.PolicyDecision)
	}
}

// ===== HTTP Proxy =====

func TestHTTPProxyAllowGetE2E(t *testing.T) {
	// Mock upstream that returns a JSON response.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "upstream-ok"})
	}))
	defer upstream.Close()

	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "http", Capability: "read", Decision: "allow"},
	}, "block")
	ev := evidence.NewSessionChain("sess-http-allow")
	aq := approval.NewQueue(100)

	services := []httpgate.ServiceConfig{
		{Name: "api", UpstreamURL: upstream.URL, PathPrefix: "/api"},
	}
	proxy := httpgate.NewProxy(pe, ev, aq, services)
	srv := httptest.NewServer(proxy)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/data")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "upstream-ok") {
		t.Errorf("expected upstream response, got %q", string(body))
	}

	// Evidence should record the allowed request.
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionAllow {
		t.Errorf("expected allow in evidence, got %s", records[0].Envelope.PolicyDecision)
	}
}

func TestHTTPProxyBlockDeleteE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "http", Capability: "read", Decision: "allow"},
		{Protocol: "http", Capability: "delete", Decision: "block"},
	}, "block")
	ev := evidence.NewSessionChain("sess-http-block")
	aq := approval.NewQueue(100)

	services := []httpgate.ServiceConfig{
		{Name: "api", UpstreamURL: "http://should-not-be-called", PathPrefix: "/api"},
	}
	proxy := httpgate.NewProxy(pe, ev, aq, services)
	srv := httptest.NewServer(proxy)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/resource/123", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for blocked DELETE, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "blocked by policy") {
		t.Errorf("expected 'blocked by policy' in response, got %q", string(body))
	}

	// Evidence should record the block.
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionBlock {
		t.Errorf("expected block in evidence, got %s", records[0].Envelope.PolicyDecision)
	}
}

func TestHTTPProxyReviewPostE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "http", Capability: "read", Decision: "allow"},
		{Protocol: "http", Capability: "write", Decision: "review"},
	}, "block")
	ev := evidence.NewSessionChain("sess-http-review")
	aq := approval.NewQueue(100)

	services := []httpgate.ServiceConfig{
		{Name: "api", UpstreamURL: "http://should-not-be-called", PathPrefix: "/api"},
	}
	proxy := httpgate.NewProxy(pe, ev, aq, services)
	srv := httptest.NewServer(proxy)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/resource", strings.NewReader(`{"name":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 for review POST, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "pending_review") {
		t.Errorf("expected 'pending_review' in response, got %q", string(body))
	}

	// Approval queue should have 1 pending item.
	pending := aq.Pending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending approval, got %d", len(pending))
	}

	// Evidence should record the review.
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionReview {
		t.Errorf("expected review in evidence, got %s", records[0].Envelope.PolicyDecision)
	}
}
