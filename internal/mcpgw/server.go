package mcpgw

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// JSON-RPC 2.0 types

// JSONRPCRequest is a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ToolCallParams represents the parameters for a tools/call request.
type ToolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// UpstreamConfig configures an upstream MCP server.
type UpstreamConfig struct {
	Name  string   `json:"name"`
	URL   string   `json:"url"`
	Tools []string `json:"tools"` // glob patterns for tool names this upstream handles
}

// Gateway is an MCP gateway proxy that intercepts tool calls for policy evaluation.
type Gateway struct {
	policyEngine *toolpolicy.Engine
	evidence     *evidence.SessionChain
	approvals    *approval.Queue
	upstreams    []UpstreamConfig
	client       *http.Client
	sse          *SSEManager
}

// NewGateway creates a new MCP gateway.
// evidence and approvals may be nil if not available.
func NewGateway(pe *toolpolicy.Engine, ev *evidence.SessionChain, aq *approval.Queue, upstreams []UpstreamConfig) *Gateway {
	return &Gateway{
		policyEngine: pe,
		evidence:     ev,
		approvals:    aq,
		upstreams:    upstreams,
		client:       &http.Client{Timeout: 30 * time.Second},
		sse:          NewSSEManager(),
	}
}

// ServeHTTP routes incoming requests to the appropriate handler.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path

	// GET /sse -> SSE stream
	if r.Method == http.MethodGet && p == "/sse" {
		g.handleSSE(w, r)
		return
	}

	// POST /mcp/session/{id} -> SSE-backed JSON-RPC
	if r.Method == http.MethodPost && strings.HasPrefix(p, "/mcp/session/") {
		sessionID := strings.TrimPrefix(p, "/mcp/session/")
		if sessionID == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}
		g.handleSessionMessage(w, r, sessionID)
		return
	}

	// POST /mcp -> direct JSON-RPC (original behaviour)
	if r.Method == http.MethodPost {
		g.handleDirectMessage(w, r)
		return
	}

	g.writeError(w, nil, -32600, "method not allowed")
}

// handleDirectMessage is the original synchronous POST /mcp handler.
func (g *Gateway) handleDirectMessage(w http.ResponseWriter, r *http.Request) {
	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		g.writeError(w, nil, -32700, "parse error")
		return
	}

	switch req.Method {
	case "tools/call":
		g.handleToolCall(w, &req)
	case "tools/list":
		g.handleToolsList(w, &req)
	case "initialize":
		g.handleInitialize(w, &req)
	case "notifications/initialized":
		w.WriteHeader(http.StatusOK)
	default:
		g.writeResult(w, req.ID, json.RawMessage(`{}`))
	}
}

// handleSSE establishes an SSE stream. The server sends an "endpoint" event
// with the URL the client should POST JSON-RPC messages to, then keeps the
// connection open and streams responses back as "message" events.
func (g *Gateway) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	session := g.sse.CreateSession()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Send the endpoint event so the client knows where to POST.
	endpointURL := fmt.Sprintf("/mcp/session/%s", session.ID)
	evt := SSEEvent{Event: "endpoint", Data: endpointURL}
	fmt.Fprint(w, evt.Format())
	flusher.Flush()

	// Keep the connection open, forwarding events until the client disconnects.
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			g.sse.RemoveSession(session.ID)
			return
		case <-session.Done:
			return
		case ev := <-session.Events:
			fmt.Fprint(w, ev.Format())
			flusher.Flush()
		}
	}
}

// handleSessionMessage receives a JSON-RPC request via POST and delivers the
// response through the corresponding SSE stream.
func (g *Gateway) handleSessionMessage(w http.ResponseWriter, r *http.Request, sessionID string) {
	session := g.sse.GetSession(sessionID)
	if session == nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "parse error", http.StatusBadRequest)
		return
	}

	// Return 202 immediately; the real response goes through SSE.
	w.WriteHeader(http.StatusAccepted)

	// Process asynchronously so we don't block the POST.
	go g.processSessionRequest(session, &req)
}

// processSessionRequest evaluates a JSON-RPC request and pushes the response
// onto the SSE session's event channel.
func (g *Gateway) processSessionRequest(session *SSESession, req *JSONRPCRequest) {
	var resp JSONRPCResponse

	switch req.Method {
	case "initialize":
		resp = g.buildInitializeResponse(req)
	case "notifications/initialized":
		// Silent acknowledgement – nothing to send back.
		return
	case "tools/call":
		resp = g.processToolCall(req)
	case "tools/list":
		resp = g.processToolsList(req)
	default:
		resp = JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{}`)}
	}

	data, _ := json.Marshal(resp)
	select {
	case session.Events <- SSEEvent{Event: "message", Data: string(data)}:
	case <-session.Done:
	}
}

// handleInitialize responds to the MCP initialize handshake over direct HTTP.
func (g *Gateway) handleInitialize(w http.ResponseWriter, req *JSONRPCRequest) {
	resp := g.buildInitializeResponse(req)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (g *Gateway) buildInitializeResponse(req *JSONRPCRequest) JSONRPCResponse {
	result, _ := json.Marshal(map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo": map[string]any{
			"name":    "aegisflow-mcp-gateway",
			"version": "0.5.0",
		},
	})
	return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

// formatUpstreamError builds a user-friendly error message for when the
// gateway cannot reach an upstream. It includes the tool name, the upstream
// name from config, the original error, and a remediation hint pointing at
// the configured URL. The URL is taken from config and is safe to expose;
// no credentials are included.
func formatUpstreamError(toolName string, upstream *UpstreamConfig, err error) string {
	return fmt.Sprintf(
		"upstream %q unreachable for tool %q: %s; check that the upstream is running at %s",
		upstream.Name, toolName, err.Error(), upstream.URL,
	)
}

// processToolCall evaluates a tools/call request and returns the response
// (used by SSE path; the direct path still calls handleToolCall).
func (g *Gateway) processToolCall(req *JSONRPCRequest) JSONRPCResponse {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: -32602, Message: "invalid params"}}
	}

	env := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "mcp-client"},
		"mcp-session",
		envelope.ProtocolMCP,
		params.Name,
		params.Name,
		inferCapability(params.Name),
	)
	env.Parameters = params.Arguments

	decision := g.policyEngine.Evaluate(env)
	env.PolicyDecision = decision
	if g.evidence != nil {
		g.evidence.Record(env)
	}

	switch decision {
	case envelope.DecisionBlock:
		log.Printf("[mcpgw] BLOCKED tool call: %s", params.Name)
		return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: -32001, Message: "tool call blocked by policy: " + params.Name}}
	case envelope.DecisionReview:
		// Check if this tool was already approved
		if g.approvals != nil && g.approvals.IsApprovedForTool(params.Name) {
			log.Printf("[mcpgw] PREVIOUSLY APPROVED tool call: %s", params.Name)
			upstream := g.findUpstream(params.Name)
			if upstream == nil {
				return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: -32003, Message: "no upstream configured for tool: " + params.Name}}
			}
			resp, err := g.proxyToUpstream(upstream, req)
			if err != nil {
				return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: -32000, Message: formatUpstreamError(params.Name, upstream, err)}}
			}
			return *resp
		}
		log.Printf("[mcpgw] REVIEW REQUIRED for tool call: %s", params.Name)
		if g.approvals != nil {
			g.approvals.Submit(env)
		}
		return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: -32002, Message: "tool call requires approval: " + params.Name}}
	case envelope.DecisionAllow:
		log.Printf("[mcpgw] ALLOWED tool call: %s", params.Name)
		upstream := g.findUpstream(params.Name)
		if upstream == nil {
			return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: -32003, Message: "no upstream configured for tool: " + params.Name}}
		}
		resp, err := g.proxyToUpstream(upstream, req)
		if err != nil {
			return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: -32000, Message: formatUpstreamError(params.Name, upstream, err)}}
		}
		return *resp
	default:
		return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &JSONRPCError{Code: -32001, Message: "unknown policy decision"}}
	}
}

// processToolsList evaluates a tools/list request and returns the response.
func (g *Gateway) processToolsList(req *JSONRPCRequest) JSONRPCResponse {
	for _, up := range g.upstreams {
		resp, err := g.proxyToUpstream(&up, req)
		if err == nil && resp.Error == nil {
			return *resp
		}
	}
	return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"tools":[]}`)}
}

func (g *Gateway) handleToolCall(w http.ResponseWriter, req *JSONRPCRequest) {
	var params ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		g.writeError(w, req.ID, -32602, "invalid params")
		return
	}

	// Build ActionEnvelope
	env := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "mcp-client"},
		"mcp-session",
		envelope.ProtocolMCP,
		params.Name,
		params.Name, // use tool name as target
		inferCapability(params.Name),
	)
	env.Parameters = params.Arguments

	// Evaluate policy
	decision := g.policyEngine.Evaluate(env)
	env.PolicyDecision = decision

	// Record in evidence chain
	if g.evidence != nil {
		g.evidence.Record(env)
	}

	switch decision {
	case envelope.DecisionBlock:
		log.Printf("[mcpgw] BLOCKED tool call: %s", params.Name)
		g.writeError(w, req.ID, -32001, "tool call blocked by policy: "+params.Name)
		return

	case envelope.DecisionReview:
		// Check if this tool was already approved
		if g.approvals != nil && g.approvals.IsApprovedForTool(params.Name) {
			log.Printf("[mcpgw] PREVIOUSLY APPROVED tool call: %s", params.Name)
			upstream := g.findUpstream(params.Name)
			if upstream == nil {
				g.writeError(w, req.ID, -32003, "no upstream configured for tool: "+params.Name)
				return
			}
			resp, err := g.proxyToUpstream(upstream, req)
			if err != nil {
				g.writeError(w, req.ID, -32000, formatUpstreamError(params.Name, upstream, err))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		log.Printf("[mcpgw] REVIEW REQUIRED for tool call: %s", params.Name)
		if g.approvals != nil {
			g.approvals.Submit(env)
		}
		g.writeError(w, req.ID, -32002, "tool call requires approval: "+params.Name)
		return

	case envelope.DecisionAllow:
		log.Printf("[mcpgw] ALLOWED tool call: %s", params.Name)
		upstream := g.findUpstream(params.Name)
		if upstream == nil {
			g.writeError(w, req.ID, -32003, "no upstream configured for tool: "+params.Name)
			return
		}

		resp, err := g.proxyToUpstream(upstream, req)
		if err != nil {
			g.writeError(w, req.ID, -32000, formatUpstreamError(params.Name, upstream, err))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return

	default:
		g.writeError(w, req.ID, -32001, "unknown policy decision")
	}
}

func (g *Gateway) handleToolsList(w http.ResponseWriter, req *JSONRPCRequest) {
	// Proxy tools/list to upstreams and return the first successful response
	for _, up := range g.upstreams {
		resp, err := g.proxyToUpstream(&up, req)
		if err == nil && resp.Error == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
	}
	// No upstreams or all failed: return empty tools list
	g.writeResult(w, req.ID, json.RawMessage(`{"tools":[]}`))
}

func (g *Gateway) findUpstream(toolName string) *UpstreamConfig {
	for i := range g.upstreams {
		for _, pattern := range g.upstreams[i].Tools {
			if matched, _ := path.Match(pattern, toolName); matched {
				return &g.upstreams[i]
			}
		}
	}
	return nil
}

func (g *Gateway) proxyToUpstream(upstream *UpstreamConfig, req *JSONRPCRequest) (*JSONRPCResponse, error) {
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequest("POST", upstream.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, err
	}
	return &rpcResp, nil
}

func (g *Gateway) writeError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (g *Gateway) writeResult(w http.ResponseWriter, id json.RawMessage, result json.RawMessage) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// inferCapability guesses the capability from the tool name.
func inferCapability(tool string) envelope.Capability {
	parts := splitToolName(tool)
	action := tool
	if len(parts) > 1 {
		action = parts[1]
	}

	readPrefixes := []string{"list_", "get_", "search_", "read_", "describe_", "show_"}
	for _, p := range readPrefixes {
		if hasPrefix(action, p) {
			return envelope.CapRead
		}
	}

	deletePrefixes := []string{"delete_", "remove_", "drop_"}
	for _, p := range deletePrefixes {
		if hasPrefix(action, p) {
			return envelope.CapDelete
		}
	}

	deployPrefixes := []string{"deploy_", "apply_", "push_"}
	for _, p := range deployPrefixes {
		if hasPrefix(action, p) {
			return envelope.CapDeploy
		}
	}

	return envelope.CapExecute
}

func splitToolName(tool string) []string {
	for i, c := range tool {
		if c == '.' {
			return []string{tool[:i], tool[i+1:]}
		}
	}
	return []string{tool}
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
