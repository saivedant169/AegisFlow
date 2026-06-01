package main

import (
	"os/exec"
	"strings"
	"testing"
)

// runBridge feeds a single stdin line to the MCP stdio bridge with the gateway
// URL pointed at a closed port, so every request exercises the unreachable
// path. Returns the bridge's stdout.
func runBridge(t *testing.T, stdinLine string) string {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not available")
	}

	cmd := exec.Command("bash", "mcp-stdio-bridge.sh")
	// 127.0.0.1:59999 is assumed closed; a short timeout keeps the test fast.
	cmd.Env = append(cmd.Environ(),
		"AEGISFLOW_MCP_URL=http://127.0.0.1:59999/mcp",
		"AEGISFLOW_MCP_TIMEOUT=2",
	)
	cmd.Stdin = strings.NewReader(stdinLine + "\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("bridge exited with error: %v", err)
	}
	return string(out)
}

// When the gateway is unreachable, a request (one with an id) must get a
// JSON-RPC error back — never an empty line, which would hang the client.
func TestMCPBridge_UnreachableReturnsError(t *testing.T) {
	out := runBridge(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)

	if strings.TrimSpace(out) == "" {
		t.Fatal("bridge returned empty output for a request; client would hang")
	}
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("expected a JSON-RPC error, got: %q", out)
	}
	if !strings.Contains(out, `"id":1`) {
		t.Fatalf("error must preserve the request id, got: %q", out)
	}
	if !strings.Contains(out, "-32000") {
		t.Fatalf("expected error code -32000, got: %q", out)
	}
}

// A string id must be preserved verbatim in the error response.
func TestMCPBridge_UnreachablePreservesStringID(t *testing.T) {
	out := runBridge(t, `{"jsonrpc":"2.0","id":"abc-7","method":"tools/list"}`)
	if !strings.Contains(out, `"id":"abc-7"`) {
		t.Fatalf("expected string id preserved, got: %q", out)
	}
}

// Notifications (no id) expect no response, so the bridge must stay silent
// even when the gateway is unreachable.
func TestMCPBridge_NotificationStaysSilent(t *testing.T) {
	out := runBridge(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if strings.TrimSpace(out) != "" {
		t.Fatalf("notification must produce no output, got: %q", out)
	}
}
