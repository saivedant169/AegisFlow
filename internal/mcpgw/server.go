package mcpgw

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"path"
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
	}
}

// ServeHTTP handles incoming JSON-RPC 2.0 requests.
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		g.writeError(w, nil, -32600, "only POST is supported")
		return
	}

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
	default:
		// Non-tool methods: return a basic success response
		g.writeResult(w, req.ID, json.RawMessage(`{}`))
	}
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
			g.writeError(w, req.ID, -32000, "upstream error: "+err.Error())
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
