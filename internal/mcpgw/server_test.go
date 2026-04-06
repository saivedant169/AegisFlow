package mcpgw

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

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

	gw := NewGateway(engine, nil, nil, nil)

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

	gw.ServeHTTP(rec, req)

	var resp JSONRPCResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error == nil {
		t.Fatal("expected error for review-required tool")
	}
	if resp.Error.Code != -32002 {
		t.Fatalf("expected review code -32002, got %d", resp.Error.Code)
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
