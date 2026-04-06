package mcpgw

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpstreamClientSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req JSONRPCRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Method != "tools/call" {
			t.Errorf("expected method tools/call, got %s", req.Method)
		}

		resp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"content":[{"type":"text","text":"ok"}]}`),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewUpstreamClient("test", server.URL, []string{"test.*"})

	resp, err := client.Send(&JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`42`),
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"test.foo","arguments":{}}`),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error in response: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Fatal("expected result")
	}
}

func TestUpstreamClientSendError(t *testing.T) {
	client := NewUpstreamClient("bad", "http://127.0.0.1:1", []string{"*"})

	_, err := client.Send(&JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "tools/list",
	})

	if err == nil {
		t.Fatal("expected error for unreachable upstream")
	}
}

func TestUpstreamClientAccessors(t *testing.T) {
	c := NewUpstreamClient("myname", "http://example.com", []string{"a.*", "b.*"})
	if c.Name() != "myname" {
		t.Errorf("expected name myname, got %s", c.Name())
	}
	if c.URL() != "http://example.com" {
		t.Errorf("expected URL http://example.com, got %s", c.URL())
	}
	if len(c.Tools()) != 2 {
		t.Errorf("expected 2 tools, got %d", len(c.Tools()))
	}
}
