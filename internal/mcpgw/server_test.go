package mcpgw

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

type failingEvidenceRecorder struct{}

func (failingEvidenceRecorder) Record(*envelope.ActionEnvelope) (*evidence.Record, error) {
	return nil, errors.New("state unavailable")
}

func TestToolCallAllowed(t *testing.T) {
	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.list_repos", Decision: "allow"},
	}, "block")

	// Mock upstream that returns a result
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`1`),
			Result:  json.RawMessage(`{"content":[{"type":"text","text":"repo1, repo2"}]}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	gw := NewGateway(engine, nil, nil, []UpstreamConfig{
		{Name: "github", URL: upstream.URL, Tools: []string{"github.*"}},
	})

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "github.list_repos",
			"arguments": {"org": "aegisflow"}
		}`),
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp JSONRPCResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}
}

func TestToolCallBlocked(t *testing.T) {
	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.delete_repo", Decision: "block"},
	}, "block")

	gw := NewGateway(engine, nil, nil, nil)

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "github.delete_repo",
			"arguments": {"repo": "important"}
		}`),
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	var resp JSONRPCResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error == nil {
		t.Fatal("expected error for blocked tool")
	}
	if resp.Error.Code != -32001 {
		t.Fatalf("expected policy error code -32001, got %d", resp.Error.Code)
	}
}

func TestToolCallReview(t *testing.T) {
	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.create_pr", Decision: "review"},
	}, "block")

	queue := approval.NewQueue(10)
	gw := NewGateway(engine, nil, queue, nil)

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "github.create_pr",
			"arguments": {"title": "fix bug"}
		}`),
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	req.Header.Set("X-API-Key", "test-agent")
	middleware.Auth(testIdentityConfig())(gw).ServeHTTP(rec, req)

	var resp JSONRPCResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error == nil {
		t.Fatal("expected error for review-required tool")
	}
	if resp.Error.Code != -32002 {
		t.Fatalf("expected review code -32002, got %d", resp.Error.Code)
	}
	data, ok := resp.Error.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected resumable review data, got %#v", resp.Error.Data)
	}
	approvalID, _ := data["approval_id"].(string)
	if approvalID == "" {
		t.Fatal("expected approval_id in direct HTTP response")
	}
	item, err := queue.Get(approvalID)
	if err != nil {
		t.Fatalf("approval_id does not identify queued action: %v", err)
	}
	if item.Envelope.Tool != "github.create_pr" {
		t.Fatalf("queued tool = %q, want github.create_pr", item.Envelope.Tool)
	}
}

func TestToolsListProxied(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`1`),
			Result: json.RawMessage(`{
				"tools": [
					{"name": "github.list_repos", "description": "List repos"},
					{"name": "github.create_pr", "description": "Create PR"}
				]
			}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	engine := toolpolicy.NewEngine(nil, "allow")
	gw := NewGateway(engine, nil, nil, []UpstreamConfig{
		{Name: "github", URL: upstream.URL, Tools: []string{"github.*"}},
	})
	defer gw.Close()

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/list",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp JSONRPCResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("expected result with tools list")
	}
}

func TestToolsListFiltersBlockedToolsOnDirectHTTP(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`1`),
			Result: json.RawMessage(`{
				"tools": [
					{"name": "github.list_repos"},
					{"name": "github.delete_repo"}
				]
			}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.delete_repo", Decision: "block"},
	}, "allow")
	gw := NewGateway(engine, nil, nil, []UpstreamConfig{
		{Name: "github", URL: upstream.URL, Tools: []string{"github.*"}},
	})
	defer gw.Close()

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/list",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if len(result.Tools) != 1 || result.Tools[0].Name != "github.list_repos" {
		t.Fatalf("expected only allowed tool, got %s", resp.Result)
	}
}

func TestToolsListSupportsAuthenticatedStreamableHTTPUpstream(t *testing.T) {
	t.Setenv("AEGISFLOW_TEST_MCP_TOKEN", "test-token")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("unexpected authorization header %q", got)
		}
		if got := r.Header.Get("Accept"); !strings.Contains(got, "text/event-stream") {
			t.Errorf("accept header must allow SSE, got %q", got)
		}
		if got := r.Header.Get("MCP-Protocol-Version"); got != "2025-03-26" {
			t.Errorf("unexpected MCP protocol version %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"tools\":[{\"name\":\"github.list_repos\"}]}}\n\n")
	}))
	defer upstream.Close()

	engine := toolpolicy.NewEngine(nil, "allow")
	gw := NewGateway(engine, nil, nil, []UpstreamConfig{{
		Name:           "github",
		URL:            upstream.URL,
		Tools:          []string{"github.*"},
		BearerTokenEnv: "AEGISFLOW_TEST_MCP_TOKEN",
	}})
	defer gw.Close()

	reqBody := JSONRPCRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/list"}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(string(resp.Result), "github.list_repos") {
		t.Fatalf("missing upstream tool in response: %s", resp.Result)
	}
}

func TestNonToolMethodPassesThrough(t *testing.T) {
	engine := toolpolicy.NewEngine(nil, "allow")
	gw := NewGateway(engine, nil, nil, nil)

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"capabilities":{}}`),
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200 for initialize, got %d", rec.Code)
	}

	var resp JSONRPCResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != nil {
		t.Fatalf("expected no error for initialize, got: %v", resp.Error)
	}
}

func TestToolCallNoUpstream(t *testing.T) {
	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "unknown.tool", Decision: "allow"},
	}, "block")

	gw := NewGateway(engine, nil, nil, nil) // no upstreams

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "unknown.tool",
			"arguments": {}
		}`),
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	var resp JSONRPCResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error == nil {
		t.Fatal("expected error for no upstream")
	}
	if resp.Error.Code != -32003 {
		t.Fatalf("expected no-upstream code -32003, got %d", resp.Error.Code)
	}
}

func TestDefaultBlockPolicy(t *testing.T) {
	// No rules, default is block
	engine := toolpolicy.NewEngine(nil, "block")
	gw := NewGateway(engine, nil, nil, nil)

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "anything.here",
			"arguments": {}
		}`),
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	var resp JSONRPCResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error == nil {
		t.Fatal("expected blocked error")
	}
	if resp.Error.Code != -32001 {
		t.Fatalf("expected -32001, got %d", resp.Error.Code)
	}
}

// TestToolCallUpstreamUnreachableErrorMessage verifies that when the MCP
// gateway cannot reach an upstream, the error message includes the tool name,
// the upstream name from config, and a remediation hint pointing at the URL.
// This is the fix for issue #82.
func TestToolCallUpstreamUnreachableErrorMessage(t *testing.T) {
	// Start a real upstream server, grab its URL, then shut it down so
	// any subsequent request fails with a connection error.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := upstream.URL
	upstream.Close()

	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.list_repos", Decision: "allow"},
	}, "block")

	gw := NewGateway(engine, nil, nil, []UpstreamConfig{
		{Name: "github-prod", URL: deadURL, Tools: []string{"github.*"}},
	})

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "github.list_repos",
			"arguments": {"org": "aegisflow"}
		}`),
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	var resp JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error for unreachable upstream, got none")
	}
	if resp.Error.Code != -32000 {
		t.Fatalf("expected upstream error code -32000, got %d", resp.Error.Code)
	}

	msg := resp.Error.Message
	if !strings.Contains(msg, "github.list_repos") {
		t.Errorf("error message should include tool name %q, got: %s", "github.list_repos", msg)
	}
	if !strings.Contains(msg, "github-prod") {
		t.Errorf("error message should include upstream name %q, got: %s", "github-prod", msg)
	}
	if !strings.Contains(msg, deadURL) {
		t.Errorf("error message should include upstream URL %q (remediation hint), got: %s", deadURL, msg)
	}
	if !strings.Contains(strings.ToLower(msg), "check that the upstream is running") {
		t.Errorf("error message should include remediation hint 'check that the upstream is running', got: %s", msg)
	}
}

func TestToolsListNoUpstreams(t *testing.T) {
	engine := toolpolicy.NewEngine(nil, "allow")
	gw := NewGateway(engine, nil, nil, nil)

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/list",
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	gw.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp JSONRPCResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
}

func TestToolCallRecordedInEvidenceChain(t *testing.T) {
	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.delete_repo", Decision: "block"},
	}, "block")
	chain := evidence.NewSessionChain("test-mcp")
	gw := NewGateway(engine, chain, nil, nil)

	reqBody := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name": "github.delete_repo", "arguments": {"repo": "important"}}`),
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	gw.ServeHTTP(httptest.NewRecorder(), req)

	if got := len(chain.Records()); got != 1 {
		t.Fatalf("expected the blocked tool call to be recorded once, got %d records", got)
	}
}

func TestToolCallFailsClosedWhenEvidenceWriteFails(t *testing.T) {
	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "repo.read", Decision: "allow"},
	}, "block")
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		json.NewEncoder(w).Encode(JSONRPCResponse{JSONRPC: "2.0", ID: json.RawMessage(`1`), Result: json.RawMessage(`{}`)})
	}))
	defer upstream.Close()

	gw := NewGateway(engine, failingEvidenceRecorder{}, nil, []UpstreamConfig{
		{Name: "repo", URL: upstream.URL, Tools: []string{"repo.*"}},
	})
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo.read","arguments":{}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	var response JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != -32004 {
		t.Fatalf("expected evidence failure -32004, got %+v", response.Error)
	}
	if upstreamCalls != 0 {
		t.Fatalf("upstream called %d times after evidence failure", upstreamCalls)
	}
}

func TestToolCallReportsApprovalQueueFailure(t *testing.T) {
	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "repo.write", Decision: "review"},
	}, "block")
	queue := approval.NewQueue(0)
	gw := NewGateway(engine, nil, queue, nil)
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo.write","arguments":{}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	req.Header.Set("X-API-Key", "test-agent")
	middleware.Auth(testIdentityConfig())(gw).ServeHTTP(rec, req)

	var response JSONRPCResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || response.Error.Code != -32005 {
		t.Fatalf("expected approval failure -32005, got %+v", response.Error)
	}
}

func TestGatewayReflectsEngineRuleChanges(t *testing.T) {
	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.delete_repo", Decision: "allow"},
	}, "block")
	gw := NewGateway(engine, nil, nil, nil)

	call := func() *JSONRPCResponse {
		body, _ := json.Marshal(JSONRPCRequest{
			JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "tools/call",
			Params: json.RawMessage(`{"name":"github.delete_repo","arguments":{}}`),
		})
		req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		gw.ServeHTTP(rec, req)
		var resp JSONRPCResponse
		json.NewDecoder(rec.Body).Decode(&resp)
		return &resp
	}

	// allow rule (no upstream configured -> -32003, not a policy block)
	if r := call(); r.Error != nil && r.Error.Code == -32001 {
		t.Fatal("expected the tool to be allowed by the initial rule")
	}

	// Simulate a hot reload swapping the rule to block.
	engine.ReplaceRules([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.delete_repo", Decision: "block"},
	}, "block")

	if r := call(); r.Error == nil || r.Error.Code != -32001 {
		t.Fatalf("expected a policy block after the rule change, got %+v", r.Error)
	}
}

func testIdentityConfig() *config.Config {
	return &config.Config{Tenants: []config.TenantConfig{{ID: "test-tenant", APIKeys: []config.APIKeyEntry{{Key: "test-agent", Role: "viewer"}}}}}
}

func TestAnonymousReviewNeverCreatesApproval(t *testing.T) {
	queue := approval.NewQueue(10)
	gateway := NewGateway(toolpolicy.NewEngine([]toolpolicy.ToolRule{{Protocol: "mcp", Tool: "repo.write", Decision: "review"}}, "block"), nil, queue, nil)
	defer gateway.Close()
	request := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo.write","arguments":{}}}`))
	response := httptest.NewRecorder()
	gateway.ServeHTTP(response, request)
	var rpc JSONRPCResponse
	if err := json.Unmarshal(response.Body.Bytes(), &rpc); err != nil {
		t.Fatal(err)
	}
	if rpc.Error == nil || rpc.Error.Code != -32006 {
		t.Fatalf("anonymous review response: %s", response.Body.String())
	}
	if len(queue.Pending()) != 0 {
		t.Fatal("anonymous approval queued")
	}
	for _, method := range []string{"GET", "HEAD"} {
		response = httptest.NewRecorder()
		middleware.Auth(testIdentityConfig())(gateway).ServeHTTP(response, httptest.NewRequest(method, "/health", nil))
		if response.Code != 200 {
			t.Fatalf("%s health: %d", method, response.Code)
		}
	}
}
