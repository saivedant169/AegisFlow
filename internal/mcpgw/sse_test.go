package mcpgw

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

func TestSSESessionCreation(t *testing.T) {
	mgr := NewSSEManager()
	s := mgr.CreateSession()
	if s == nil {
		t.Fatal("expected session, got nil")
	}
	if s.ID == "" {
		t.Fatal("expected non-empty session ID")
	}
	if mgr.GetSession(s.ID) == nil {
		t.Fatal("session not retrievable after creation")
	}
}

func TestSSEEndpointEvent(t *testing.T) {
	engine := toolpolicy.NewEngine(nil, "allow")
	gw := NewGateway(engine, nil, nil, nil)

	srv := httptest.NewServer(gw)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/sse")
	if err != nil {
		t.Fatalf("GET /sse failed: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	var eventType, data string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
		}
		if line == "" && eventType != "" {
			break
		}
	}

	if eventType != "endpoint" {
		t.Fatalf("expected event type 'endpoint', got %q", eventType)
	}
	if !strings.HasPrefix(data, "/mcp/session/") {
		t.Fatalf("expected endpoint URL starting with /mcp/session/, got %q", data)
	}
}

func TestSSEToolCallViaSession(t *testing.T) {
	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.list_repos", Decision: "allow"},
	}, "block")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      json.RawMessage(`1`),
			Result:  json.RawMessage(`{"content":[{"type":"text","text":"repo1"}]}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	gw := NewGateway(engine, nil, nil, []UpstreamConfig{
		{Name: "github", URL: upstream.URL, Tools: []string{"github.*"}},
	})

	srv := httptest.NewServer(gw)
	defer srv.Close()

	// Connect SSE stream.
	sseResp, err := http.Get(srv.URL + "/sse")
	if err != nil {
		t.Fatalf("GET /sse failed: %v", err)
	}
	defer sseResp.Body.Close()

	// Read endpoint event.
	sessionURL := readEndpointEvent(t, sseResp)

	// POST a tool call.
	reqBody, _ := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"github.list_repos","arguments":{"org":"test"}}`),
	})
	postResp, err := http.Post(srv.URL+sessionURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST to session failed: %v", err)
	}
	if postResp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d", postResp.StatusCode)
	}
	postResp.Body.Close()

	// Read the response from the SSE stream.
	rpcResp := readMessageEvent(t, sseResp)
	if rpcResp.Error != nil {
		t.Fatalf("expected no error, got: %s", rpcResp.Error.Message)
	}
	if rpcResp.Result == nil {
		t.Fatal("expected result, got nil")
	}
}

func TestSSEToolCallBlocked(t *testing.T) {
	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.delete_repo", Decision: "block"},
	}, "block")

	gw := NewGateway(engine, nil, nil, nil)
	srv := httptest.NewServer(gw)
	defer srv.Close()

	sseResp, err := http.Get(srv.URL + "/sse")
	if err != nil {
		t.Fatalf("GET /sse failed: %v", err)
	}
	defer sseResp.Body.Close()

	sessionURL := readEndpointEvent(t, sseResp)

	reqBody, _ := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"github.delete_repo","arguments":{"repo":"important"}}`),
	})
	postResp, err := http.Post(srv.URL+sessionURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	postResp.Body.Close()

	rpcResp := readMessageEvent(t, sseResp)
	if rpcResp.Error == nil {
		t.Fatal("expected error for blocked tool")
	}
	if rpcResp.Error.Code != -32001 {
		t.Fatalf("expected -32001, got %d", rpcResp.Error.Code)
	}
}

func TestSSESessionCleanup(t *testing.T) {
	mgr := NewSSEManager()
	s1 := mgr.CreateSession()
	s1.Created = time.Now().Add(-2 * time.Hour)
	s2 := mgr.CreateSession()

	mgr.CleanupStale(1 * time.Hour)

	if mgr.GetSession(s1.ID) != nil {
		t.Fatal("stale session should have been removed")
	}
	if mgr.GetSession(s2.ID) == nil {
		t.Fatal("fresh session should still exist")
	}
}

func TestSSEInitializeHandshake(t *testing.T) {
	engine := toolpolicy.NewEngine(nil, "allow")
	gw := NewGateway(engine, nil, nil, nil)
	srv := httptest.NewServer(gw)
	defer srv.Close()

	sseResp, err := http.Get(srv.URL + "/sse")
	if err != nil {
		t.Fatalf("GET /sse failed: %v", err)
	}
	defer sseResp.Body.Close()

	sessionURL := readEndpointEvent(t, sseResp)

	reqBody, _ := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{"capabilities":{}}`),
	})
	postResp, err := http.Post(srv.URL+sessionURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	postResp.Body.Close()

	rpcResp := readMessageEvent(t, sseResp)
	if rpcResp.Error != nil {
		t.Fatalf("expected no error, got: %s", rpcResp.Error.Message)
	}

	var result map[string]any
	if err := json.Unmarshal(rpcResp.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("unexpected protocolVersion: %v", result["protocolVersion"])
	}
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatal("missing serverInfo")
	}
	if serverInfo["name"] != "aegisflow-mcp-gateway" {
		t.Fatalf("unexpected server name: %v", serverInfo["name"])
	}
}

func TestSSEMultipleRequestsOnOneSession(t *testing.T) {
	engine := toolpolicy.NewEngine(nil, "allow")
	gw := NewGateway(engine, nil, nil, nil)
	srv := httptest.NewServer(gw)
	defer srv.Close()

	sseResp, err := http.Get(srv.URL + "/sse")
	if err != nil {
		t.Fatalf("GET /sse failed: %v", err)
	}
	defer sseResp.Body.Close()

	sessionURL := readEndpointEvent(t, sseResp)

	// Send initialize.
	initBody, _ := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(`{}`),
	})
	r1, _ := http.Post(srv.URL+sessionURL, "application/json", bytes.NewReader(initBody))
	r1.Body.Close()

	resp1 := readMessageEvent(t, sseResp)
	if resp1.Error != nil {
		t.Fatalf("initialize error: %s", resp1.Error.Message)
	}

	// Send tools/list.
	listBody, _ := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`2`),
		Method:  "tools/list",
	})
	r2, _ := http.Post(srv.URL+sessionURL, "application/json", bytes.NewReader(listBody))
	r2.Body.Close()

	resp2 := readMessageEvent(t, sseResp)
	if resp2.Error != nil {
		t.Fatalf("tools/list error: %s", resp2.Error.Message)
	}

	// Verify IDs match their respective requests.
	if string(resp1.ID) != "1" {
		t.Fatalf("expected ID 1 on first response, got %s", string(resp1.ID))
	}
	if string(resp2.ID) != "2" {
		t.Fatalf("expected ID 2 on second response, got %s", string(resp2.ID))
	}
}

// --- helpers ---

// readEndpointEvent reads lines from the SSE stream until it finds the
// "endpoint" event and returns the data (the session URL).
func readEndpointEvent(t *testing.T, resp *http.Response) string {
	t.Helper()
	scanner := bufio.NewScanner(resp.Body)
	var eventType, data string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
		}
		if line == "" && eventType == "endpoint" {
			return data
		}
	}
	t.Fatal("did not receive endpoint event")
	return ""
}

// readMessageEvent reads lines from the SSE stream until it finds a "message"
// event and returns the parsed JSON-RPC response.
func readMessageEvent(t *testing.T, resp *http.Response) JSONRPCResponse {
	t.Helper()
	scanner := bufio.NewScanner(resp.Body)
	var eventType, data string
	deadline := time.After(5 * time.Second)
	ch := make(chan JSONRPCResponse, 1)

	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
			}
			if strings.HasPrefix(line, "data: ") {
				data = strings.TrimPrefix(line, "data: ")
			}
			if line == "" && eventType == "message" && data != "" {
				var rpc JSONRPCResponse
				json.Unmarshal([]byte(data), &rpc)
				ch <- rpc
				return
			}
		}
	}()

	select {
	case r := <-ch:
		return r
	case <-deadline:
		t.Fatal("timed out waiting for message event")
		return JSONRPCResponse{}
	}
}
