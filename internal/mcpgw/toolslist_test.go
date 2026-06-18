package mcpgw

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// tools/list must not advertise a tool the policy would block.
func TestFilterToolsByPolicy_DropsBlocked(t *testing.T) {
	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "github.delete_repo", Decision: "block"},
	}, "allow")
	gw := NewGateway(engine, nil, nil, nil)

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Result:  json.RawMessage(`{"tools":[{"name":"github.list_repos"},{"name":"github.delete_repo"}],"nextCursor":"x"}`),
	}
	out := gw.filterToolsByPolicy(resp.ID, resp)

	var payload struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
		NextCursor string `json:"nextCursor"`
	}
	if err := json.Unmarshal(out.Result, &payload); err != nil {
		t.Fatalf("decode: %v (%s)", err, out.Result)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Name != "github.list_repos" {
		t.Fatalf("expected only the allowed tool, got %s", out.Result)
	}
	if payload.NextCursor != "x" {
		t.Errorf("other result fields must be preserved, got %s", out.Result)
	}
	if strings.Contains(string(out.Result), "delete_repo") {
		t.Errorf("blocked tool still advertised: %s", out.Result)
	}
}

// With no policy engine, the list passes through unchanged.
func TestFilterToolsByPolicy_NoEnginePassThrough(t *testing.T) {
	gw := &Gateway{}
	resp := JSONRPCResponse{ID: json.RawMessage(`1`), Result: json.RawMessage(`{"tools":[{"name":"x"}]}`)}
	out := gw.filterToolsByPolicy(resp.ID, resp)
	if string(out.Result) != string(resp.Result) {
		t.Errorf("expected pass-through, got %s", out.Result)
	}
}
