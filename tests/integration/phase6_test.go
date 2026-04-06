package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/mcpgw"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// helper: build a JSON-RPC tool call request body.
func toolCallBody(t *testing.T, id int, toolName string, args map[string]any) []byte {
	t.Helper()
	params, _ := json.Marshal(map[string]any{"name": toolName, "arguments": args})
	req := mcpgw.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(jsonInt(id)),
		Method:  "tools/call",
		Params:  params,
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return b
}

func jsonInt(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// helper: POST to a gateway test server.
func gwPost(t *testing.T, url string, body []byte) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	return resp
}

// helper: decode JSON-RPC response.
func decodeRPC(t *testing.T, resp *http.Response) mcpgw.JSONRPCResponse {
	t.Helper()
	defer resp.Body.Close()
	var rpc mcpgw.JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		t.Fatalf("decode JSON-RPC response: %v", err)
	}
	return rpc
}

// mockUpstream returns a test server that echoes a successful JSON-RPC result.
func mockUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := mcpgw.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`1`),
			Result:  json.RawMessage(`{"content":[{"type":"text","text":"upstream-ok"}]}`),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
}

// ---------- Tests ----------

func TestMCPToolCallAllowedE2E(t *testing.T) {
	upstream := mockUpstream(t)
	defer upstream.Close()

	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.list_repos", Decision: "allow"},
	}, "block")
	ev := evidence.NewSessionChain("sess-allow")
	aq := approval.NewQueue(100)

	gw := mcpgw.NewGateway(pe, ev, aq, []mcpgw.UpstreamConfig{
		{Name: "github", URL: upstream.URL, Tools: []string{"github.*"}},
	})
	srv := httptest.NewServer(gw)
	defer srv.Close()

	body := toolCallBody(t, 1, "github.list_repos", map[string]any{"org": "test"})
	resp := gwPost(t, srv.URL, body)
	rpc := decodeRPC(t, resp)

	if rpc.Error != nil {
		t.Fatalf("expected no error, got code=%d msg=%s", rpc.Error.Code, rpc.Error.Message)
	}
	if rpc.Result == nil {
		t.Fatal("expected result from upstream, got nil")
	}
	// Verify the upstream response was proxied through.
	if !bytes.Contains(rpc.Result, []byte("upstream-ok")) {
		t.Errorf("expected upstream result in response, got %s", string(rpc.Result))
	}

	// Verify evidence was recorded.
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.Tool != "github.list_repos" {
		t.Errorf("expected tool github.list_repos, got %s", records[0].Envelope.Tool)
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionAllow {
		t.Errorf("expected decision allow, got %s", records[0].Envelope.PolicyDecision)
	}
}

func TestMCPToolCallBlockedE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.delete_repo", Decision: "block"},
	}, "block")
	ev := evidence.NewSessionChain("sess-block")
	aq := approval.NewQueue(100)

	gw := mcpgw.NewGateway(pe, ev, aq, nil)
	srv := httptest.NewServer(gw)
	defer srv.Close()

	body := toolCallBody(t, 1, "github.delete_repo", map[string]any{"repo": "important"})
	resp := gwPost(t, srv.URL, body)
	rpc := decodeRPC(t, resp)

	if rpc.Error == nil {
		t.Fatal("expected error for blocked tool")
	}
	if rpc.Error.Code != -32001 {
		t.Fatalf("expected error code -32001, got %d", rpc.Error.Code)
	}

	// Verify evidence records the block.
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionBlock {
		t.Errorf("expected decision block, got %s", records[0].Envelope.PolicyDecision)
	}
}

func TestMCPToolCallReviewE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.create_pr", Decision: "review"},
	}, "block")
	ev := evidence.NewSessionChain("sess-review")
	aq := approval.NewQueue(100)

	gw := mcpgw.NewGateway(pe, ev, aq, nil)
	srv := httptest.NewServer(gw)
	defer srv.Close()

	body := toolCallBody(t, 1, "github.create_pr", map[string]any{"title": "fix bug"})
	resp := gwPost(t, srv.URL, body)
	rpc := decodeRPC(t, resp)

	if rpc.Error == nil {
		t.Fatal("expected error for review-required tool")
	}
	if rpc.Error.Code != -32002 {
		t.Fatalf("expected error code -32002, got %d", rpc.Error.Code)
	}

	// Verify action is queued in approval queue.
	pending := aq.Pending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending approval, got %d", len(pending))
	}
	if pending[0].Envelope.Tool != "github.create_pr" {
		t.Errorf("expected tool github.create_pr in queue, got %s", pending[0].Envelope.Tool)
	}

	// Verify evidence records the review decision.
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionReview {
		t.Errorf("expected decision review, got %s", records[0].Envelope.PolicyDecision)
	}
}

func TestApprovalWorkflowE2E(t *testing.T) {
	aq := approval.NewQueue(100)

	// Submit an envelope directly.
	env := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "test-agent"},
		"test-task",
		envelope.ProtocolMCP,
		"github.create_pr",
		"github.create_pr",
		envelope.CapExecute,
	)
	env.PolicyDecision = envelope.DecisionReview

	id, err := aq.Submit(env)
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// List pending -- should have 1.
	pending := aq.Pending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].Status != approval.StatusPending {
		t.Errorf("expected status pending, got %s", pending[0].Status)
	}

	// Approve it.
	item, err := aq.Approve(id, "admin-user", "looks good")
	if err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if item.Status != approval.StatusApproved {
		t.Errorf("expected status approved, got %s", item.Status)
	}
	if item.Reviewer != "admin-user" {
		t.Errorf("expected reviewer admin-user, got %s", item.Reviewer)
	}

	// Pending should now be empty.
	pending = aq.Pending()
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending after approval, got %d", len(pending))
	}

	// History should have 1.
	history := aq.History(10)
	if len(history) != 1 {
		t.Fatalf("expected 1 in history, got %d", len(history))
	}
	if history[0].Status != approval.StatusApproved {
		t.Errorf("expected approved in history, got %s", history[0].Status)
	}
}

func TestDenyWorkflowE2E(t *testing.T) {
	aq := approval.NewQueue(100)

	env := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "test-agent"},
		"test-task",
		envelope.ProtocolMCP,
		"github.delete_repo",
		"github.delete_repo",
		envelope.CapDelete,
	)
	env.PolicyDecision = envelope.DecisionReview

	id, err := aq.Submit(env)
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}

	// Deny it.
	item, err := aq.Deny(id, "security-admin", "too dangerous")
	if err != nil {
		t.Fatalf("deny failed: %v", err)
	}
	if item.Status != approval.StatusDenied {
		t.Errorf("expected status denied, got %s", item.Status)
	}
	if item.ReviewComment != "too dangerous" {
		t.Errorf("expected comment 'too dangerous', got %s", item.ReviewComment)
	}

	// Pending should be empty.
	pending := aq.Pending()
	if len(pending) != 0 {
		t.Fatalf("expected 0 pending after denial, got %d", len(pending))
	}

	// History should record the denial.
	history := aq.History(10)
	if len(history) != 1 {
		t.Fatalf("expected 1 in history, got %d", len(history))
	}
	if history[0].Status != approval.StatusDenied {
		t.Errorf("expected denied in history, got %s", history[0].Status)
	}
}

func TestEvidenceChainIntegrityE2E(t *testing.T) {
	chain := evidence.NewSessionChain("sess-integrity")

	// Record allow action.
	envAllow := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "a1"},
		"task1", envelope.ProtocolMCP,
		"github.list_repos", "github.list_repos", envelope.CapRead,
	)
	envAllow.PolicyDecision = envelope.DecisionAllow
	chain.Record(envAllow)

	// Record block action.
	envBlock := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "a1"},
		"task1", envelope.ProtocolMCP,
		"github.delete_repo", "github.delete_repo", envelope.CapDelete,
	)
	envBlock.PolicyDecision = envelope.DecisionBlock
	chain.Record(envBlock)

	// Record review action.
	envReview := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "a1"},
		"task1", envelope.ProtocolMCP,
		"github.create_pr", "github.create_pr", envelope.CapExecute,
	)
	envReview.PolicyDecision = envelope.DecisionReview
	chain.Record(envReview)

	// Export and verify chain.
	exportData, err := chain.Export()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if len(exportData) == 0 {
		t.Fatal("export returned empty data")
	}

	records := chain.Records()
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	result := evidence.Verify(records)
	if !result.Valid {
		t.Fatalf("evidence chain verification failed: %s", result.Message)
	}
	if result.TotalRecords != 3 {
		t.Errorf("expected 3 total records in result, got %d", result.TotalRecords)
	}
}

func TestEvidenceChainTamperDetectionE2E(t *testing.T) {
	chain := evidence.NewSessionChain("sess-tamper")

	env1 := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "a1"},
		"task1", envelope.ProtocolMCP,
		"github.list_repos", "github.list_repos", envelope.CapRead,
	)
	env1.PolicyDecision = envelope.DecisionAllow
	chain.Record(env1)

	env2 := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "a1"},
		"task1", envelope.ProtocolMCP,
		"github.create_pr", "github.create_pr", envelope.CapExecute,
	)
	env2.PolicyDecision = envelope.DecisionReview
	chain.Record(env2)

	records := chain.Records()

	// Verify chain is valid before tampering.
	result := evidence.Verify(records)
	if !result.Valid {
		t.Fatalf("chain should be valid before tampering: %s", result.Message)
	}

	// Tamper with the first record's envelope tool name.
	records[0].Envelope.Tool = "TAMPERED"

	result = evidence.Verify(records)
	if result.Valid {
		t.Fatal("expected verification to fail after tampering, but it passed")
	}
	if result.ErrorAtIndex != 0 {
		t.Errorf("expected error at index 0, got %d", result.ErrorAtIndex)
	}
}

func TestToolPolicyGlobMatchingE2E(t *testing.T) {
	upstream := mockUpstream(t)
	defer upstream.Close()

	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.list_*", Decision: "allow"},
		{Protocol: "mcp", Tool: "github.create_*", Decision: "review"},
		{Protocol: "mcp", Tool: "github.delete_*", Decision: "block"},
	}, "block")
	ev := evidence.NewSessionChain("sess-glob")

	gw := mcpgw.NewGateway(pe, ev, approval.NewQueue(100), []mcpgw.UpstreamConfig{
		{Name: "github", URL: upstream.URL, Tools: []string{"github.*"}},
	})
	srv := httptest.NewServer(gw)
	defer srv.Close()

	tests := []struct {
		tool         string
		expectedCode int // 0 means no error (allowed), otherwise JSON-RPC error code
	}{
		{"github.list_repos", 0},
		{"github.list_issues", 0},
		{"github.create_pr", -32002},
		{"github.create_issue", -32002},
		{"github.delete_repo", -32001},
		{"github.delete_branch", -32001},
	}

	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			body := toolCallBody(t, 1, tc.tool, map[string]any{})
			resp := gwPost(t, srv.URL, body)
			rpc := decodeRPC(t, resp)

			if tc.expectedCode == 0 {
				if rpc.Error != nil {
					t.Fatalf("expected no error for %s, got code=%d msg=%s",
						tc.tool, rpc.Error.Code, rpc.Error.Message)
				}
			} else {
				if rpc.Error == nil {
					t.Fatalf("expected error code %d for %s, got success", tc.expectedCode, tc.tool)
				}
				if rpc.Error.Code != tc.expectedCode {
					t.Fatalf("expected error code %d for %s, got %d",
						tc.expectedCode, tc.tool, rpc.Error.Code)
				}
			}
		})
	}

	// Verify evidence recorded all 6 calls.
	records := ev.Records()
	if len(records) != 6 {
		t.Fatalf("expected 6 evidence records, got %d", len(records))
	}
}

func TestFullLifecycleE2E(t *testing.T) {
	upstream := mockUpstream(t)
	defer upstream.Close()

	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.list_*", Decision: "allow"},
		{Protocol: "mcp", Tool: "github.delete_*", Decision: "block"},
		{Protocol: "mcp", Tool: "github.create_*", Decision: "review"},
	}, "block")
	ev := evidence.NewSessionChain("sess-lifecycle")
	aq := approval.NewQueue(100)

	gw := mcpgw.NewGateway(pe, ev, aq, []mcpgw.UpstreamConfig{
		{Name: "github", URL: upstream.URL, Tools: []string{"github.*"}},
	})
	srv := httptest.NewServer(gw)
	defer srv.Close()

	// Step 1: Allowed read.
	body := toolCallBody(t, 1, "github.list_repos", map[string]any{"org": "acme"})
	resp := gwPost(t, srv.URL, body)
	rpc := decodeRPC(t, resp)
	if rpc.Error != nil {
		t.Fatalf("step 1 (allow): expected no error, got code=%d msg=%s", rpc.Error.Code, rpc.Error.Message)
	}

	// Step 2: Blocked delete.
	body = toolCallBody(t, 2, "github.delete_repo", map[string]any{"repo": "critical"})
	resp = gwPost(t, srv.URL, body)
	rpc = decodeRPC(t, resp)
	if rpc.Error == nil || rpc.Error.Code != -32001 {
		t.Fatalf("step 2 (block): expected -32001, got %+v", rpc.Error)
	}

	// Step 3: Review-required write.
	body = toolCallBody(t, 3, "github.create_issue", map[string]any{"title": "new feature"})
	resp = gwPost(t, srv.URL, body)
	rpc = decodeRPC(t, resp)
	if rpc.Error == nil || rpc.Error.Code != -32002 {
		t.Fatalf("step 3 (review): expected -32002, got %+v", rpc.Error)
	}

	// Step 4: Approve the pending item.
	pending := aq.Pending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending approval, got %d", len(pending))
	}
	approvedItem, err := aq.Approve(pending[0].ID, "lead-dev", "approved for release")
	if err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if approvedItem.Status != approval.StatusApproved {
		t.Errorf("expected approved status, got %s", approvedItem.Status)
	}

	// Step 5: Verify evidence chain has all 3 actions with correct decisions.
	records := ev.Records()
	if len(records) != 3 {
		t.Fatalf("expected 3 evidence records, got %d", len(records))
	}

	expectedTools := []string{"github.list_repos", "github.delete_repo", "github.create_issue"}
	expectedDecisions := []envelope.Decision{envelope.DecisionAllow, envelope.DecisionBlock, envelope.DecisionReview}

	for i, rec := range records {
		if rec.Envelope.Tool != expectedTools[i] {
			t.Errorf("record %d: expected tool %s, got %s", i, expectedTools[i], rec.Envelope.Tool)
		}
		if rec.Envelope.PolicyDecision != expectedDecisions[i] {
			t.Errorf("record %d: expected decision %s, got %s", i, expectedDecisions[i], rec.Envelope.PolicyDecision)
		}
	}

	// Verify chain integrity.
	result := evidence.Verify(records)
	if !result.Valid {
		t.Fatalf("evidence chain verification failed: %s", result.Message)
	}
}

func TestMCPGatewayNoUpstreamE2E(t *testing.T) {
	pe := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "unknown.tool", Decision: "allow"},
	}, "block")
	ev := evidence.NewSessionChain("sess-no-upstream")

	gw := mcpgw.NewGateway(pe, ev, nil, nil) // no upstreams
	srv := httptest.NewServer(gw)
	defer srv.Close()

	body := toolCallBody(t, 1, "unknown.tool", map[string]any{})
	resp := gwPost(t, srv.URL, body)
	rpc := decodeRPC(t, resp)

	if rpc.Error == nil {
		t.Fatal("expected error for no upstream")
	}
	if rpc.Error.Code != -32003 {
		t.Fatalf("expected error code -32003, got %d", rpc.Error.Code)
	}

	// Evidence should still record the allow decision (upstream failure happens after policy).
	records := ev.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 evidence record, got %d", len(records))
	}
	if records[0].Envelope.PolicyDecision != envelope.DecisionAllow {
		t.Errorf("expected decision allow, got %s", records[0].Envelope.PolicyDecision)
	}
}
