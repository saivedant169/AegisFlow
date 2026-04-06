package mcpgw

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// UpstreamClient communicates with a single upstream MCP server.
type UpstreamClient struct {
	name   string
	url    string
	tools  []string
	client *http.Client
}

// NewUpstreamClient creates a client for a specific upstream MCP server.
func NewUpstreamClient(name, url string, tools []string) *UpstreamClient {
	return &UpstreamClient{
		name:  name,
		url:   url,
		tools: tools,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Send forwards a JSON-RPC request to the upstream and returns the response.
func (u *UpstreamClient) Send(req *JSONRPCRequest) (*JSONRPCResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", u.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := u.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("upstream %s: %w", u.name, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response from %s: %w", u.name, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %s returned status %d: %s", u.name, resp.StatusCode, string(respBody))
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal response from %s: %w", u.name, err)
	}

	return &rpcResp, nil
}

// Name returns the upstream name.
func (u *UpstreamClient) Name() string { return u.name }

// URL returns the upstream URL.
func (u *UpstreamClient) URL() string { return u.url }

// Tools returns the tool patterns this upstream handles.
func (u *UpstreamClient) Tools() []string { return u.tools }
