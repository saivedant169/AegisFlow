package admin

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/mcpgw"
	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

func TestMCPAuthenticatedBoundary(t *testing.T) {
	cfg := &config.Config{Tenants: []config.TenantConfig{
		{ID: "tenant-a", APIKeys: []config.APIKeyEntry{{Key: "agent-a", Role: "viewer"}, {Key: "peer-a", Role: "viewer"}, {Key: "reviewer-a", Role: "operator"}}},
		{ID: "tenant-b", APIKeys: []config.APIKeyEntry{{Key: "agent-b", Role: "viewer"}, {Key: "reviewer-b", Role: "operator"}}},
	}}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, err := io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`)
		if err != nil {
			return
		}
	}))
	defer upstream.Close()
	queue := approval.NewQueue(1000)
	registry := evidence.NewChainRegistry([]byte("test-signing-key"))
	defer registry.Close()
	gateway := mcpgw.NewGateway(toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "mcp", Tool: "repo.write", Decision: "review"}, {Protocol: "mcp", Tool: "repo.read", Decision: "allow"},
	}, "block"), registry, queue, []mcpgw.UpstreamConfig{{Name: "repo", URL: upstream.URL, Tools: []string{"repo.*"}}})
	defer gateway.Close()
	mcp := httptest.NewServer(middleware.Auth(cfg)(gateway))
	defer mcp.Close()
	server := newIntegrationAdminServer()
	server.cfg = cfg
	server.approvalProvider = approval.NewAdminAdapter(queue)
	server.evidenceProvider = evidence.NewRegistryAdminAdapter(registry)
	admin := httptest.NewServer(server.Router())
	defer admin.Close()
	callBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repo.write","arguments":{"target":"main"}}}`
	request := func(method, url, key, session, body string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, url, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-API-Key", key)
		req.Header.Set("X-AegisFlow-Session-ID", session)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				t.Fatal(err)
			}
		}(resp.Body)
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, data
	}
	pending := func(data []byte) string {
		t.Helper()
		var rpc mcpgw.JSONRPCResponse
		if err := json.Unmarshal(data, &rpc); err != nil {
			t.Fatal(err)
		}
		if rpc.Error == nil || rpc.Error.Code != -32002 {
			t.Fatalf("expected review, got %s", data)
		}
		return rpc.Error.Data.(map[string]any)["approval_id"].(string)
	}
	for _, route := range []string{"/mcp", "/health", "/unknown", "/mcp/session/unknown"} {
		for _, key := range []string{"", "invalid"} {
			status, _ := request("POST", mcp.URL+route, key, "", callBody)
			if status != 401 {
				t.Errorf("unauthenticated %s: %d", route, status)
			}
		}
	}
	for _, route := range []string{"/health", "/unknown"} {
		status, _ := request("POST", mcp.URL+route, "agent-a", "", callBody)
		if status != 404 {
			t.Errorf("unknown MCP route %s: %d", route, status)
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid route reached upstream")
	}
	_, body := request("POST", mcp.URL+"/mcp", "agent-a", "session-1", callBody)
	id := pending(body)
	for _, route := range []string{"/approvals", "/approvals/history", "/approvals/" + id, "/evidence/sessions"} {
		status, _ := request("GET", admin.URL+"/admin/v1"+route, "", "", "")
		if status != 401 {
			t.Errorf("anonymous admin %s: %d", route, status)
		}
	}
	for _, key := range []string{"agent-a", "agent-b", "peer-a"} {
		status, _ := request("POST", admin.URL+"/admin/v1/approvals/"+id+"/approve", key, "", `{"reviewer":"reviewer-a"}`)
		if status != 403 {
			t.Errorf("viewer review %s: %d", key, status)
		}
	}
	status, _ := request("GET", admin.URL+"/admin/v1/approvals/"+id, "agent-b", "", "")
	if status != 404 {
		t.Fatalf("cross-tenant approval read: %d", status)
	}
	status, _ = request("POST", admin.URL+"/admin/v1/approvals/"+id+"/approve", "reviewer-b", "", `{}`)
	if status != 400 {
		t.Fatalf("cross-tenant review: %d", status)
	}
	status, body = request("POST", admin.URL+"/admin/v1/approvals/"+id+"/approve", "reviewer-a", "", `{"reviewer":"forged-user"}`)
	if status != 200 || bytes.Contains(body, []byte("forged-user")) || !bytes.Contains(body, []byte("principal-v1-")) {
		t.Fatalf("review identity: %d %s", status, body)
	}
	for _, attempt := range [][2]string{{"agent-b", "session-1"}, {"peer-a", "session-1"}, {"agent-a", "session-2"}} {
		_, body = request("POST", mcp.URL+"/mcp", attempt[0], attempt[1], callBody)
		pending(body)
	}
	_, body = request("POST", mcp.URL+"/mcp", "agent-a", "session-1", strings.Replace(callBody, "main", "other", 1))
	pending(body)
	if calls.Load() != 0 {
		t.Fatal("approval escaped caller or argument scope")
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() { request("POST", mcp.URL+"/mcp", "agent-a", "session-1", callBody) })
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("approval used %d times", calls.Load())
	}
	_, body = request("GET", admin.URL+"/admin/v1/evidence/sessions", "agent-a", "", "")
	var sessions []evidence.SessionManifest
	if err := json.Unmarshal(body, &sessions); err != nil || len(sessions) == 0 {
		t.Fatalf("sessions: %s %v", body, err)
	}
	for _, session := range sessions {
		for _, route := range []string{"export", "verify", "report", "report.html"} {
			method := "GET"
			if route == "verify" {
				method = "POST"
			}
			url := fmt.Sprintf("%s/admin/v1/evidence/sessions/%s/%s", admin.URL, session.SessionID, route)
			status, _ = request(method, url, "agent-b", "", "")
			if status != 404 {
				t.Errorf("cross-tenant evidence %s: %d", route, status)
			}
			status, _ = request(method, url, "agent-a", "", "")
			if status != 200 {
				t.Errorf("own evidence %s: %d", route, status)
			}
		}
	}
	status, _ = request("GET", mcp.URL+"/sse", "", "", "")
	if status != 401 {
		t.Fatalf("anonymous SSE: %d", status)
	}
	// SSE endpoint knowledge cannot transfer ownership, even within one tenant.
	req, _ := http.NewRequest("GET", mcp.URL+"/sse", nil)
	req.Header.Set("X-API-Key", "agent-a")
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			t.Fatal(err)
		}
	}(stream.Body)
	scanner := bufio.NewScanner(stream.Body)
	endpoint := ""
	for scanner.Scan() {
		if after, ok :=strings.CutPrefix(scanner.Text(), "data: "); ok  {
			endpoint = after
			break
		}
	}
	if endpoint == "" {
		t.Fatal("missing SSE endpoint")
	}
	for _, key := range []string{"agent-b", "peer-a"} {
		status, _ = request("POST", mcp.URL+endpoint, key, "", callBody)
		if status != 404 {
			t.Errorf("SSE hijack %s: %d", key, status)
		}
	}
	status, _ = request("POST", mcp.URL+endpoint, "agent-a", "", callBody)
	if status != 202 {
		t.Fatalf("SSE owner: %d", status)
	}
	for scanner.Scan() {
		if after, ok :=strings.CutPrefix(scanner.Text(), "data: "); ok  {
			id = pending([]byte(after))
			break
		}
	}
	if calls.Load() != 1 {
		t.Fatal("SSE consumed HTTP approval")
	}
	status, body = request("POST", admin.URL+"/admin/v1/approvals/"+id+"/approve", "reviewer-a", "", `{}`)
	if status != 200 {
		t.Fatalf("SSE review: %d %s", status, body)
	}
	status, _ = request("POST", mcp.URL+endpoint, "agent-a", "", callBody)
	if status != 202 {
		t.Fatalf("SSE retry: %d", status)
	}
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "data: ") {
			var rpc mcpgw.JSONRPCResponse
			if err := json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data: ")), &rpc); err != nil {
				t.Fatal(err)
			}
			if rpc.Error != nil {
				t.Fatalf("SSE approved retry: %+v", rpc.Error)
			}
			break
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("SSE execution count: %d", calls.Load())
	}
}
