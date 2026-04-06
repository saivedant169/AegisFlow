package httpgate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// helper to create a proxy with given rules and an upstream mock server.
func setupProxy(t *testing.T, rules []toolpolicy.ToolRule, defaultDecision string, upstream *httptest.Server) (*Proxy, []ServiceConfig) {
	t.Helper()
	engine := toolpolicy.NewEngine(rules, defaultDecision)
	chain := evidence.NewSessionChain("test-session")
	queue := approval.NewQueue(100)

	upstreamURL := ""
	if upstream != nil {
		upstreamURL = upstream.URL
	}

	services := []ServiceConfig{
		{Name: "stripe", UpstreamURL: upstreamURL, PathPrefix: "/stripe"},
		{Name: "slack", UpstreamURL: upstreamURL, PathPrefix: "/slack"},
	}

	proxy := NewProxy(engine, chain, queue, services)
	return proxy, services
}

func TestAllowGetRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	rules := []toolpolicy.ToolRule{
		{Protocol: "http", Capability: "read", Decision: "allow"},
	}
	proxy, _ := setupProxy(t, rules, "block", upstream)

	req := httptest.NewRequest("GET", "/stripe/v1/charges", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestBlockDeleteRequest(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "http", Capability: "delete", Decision: "block"},
	}
	proxy, _ := setupProxy(t, rules, "block", nil)

	req := httptest.NewRequest("DELETE", "/stripe/v1/customers/cus_123", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["error"] != "request blocked by policy" {
		t.Errorf("unexpected error message: %s", body["error"])
	}
}

func TestReviewPostRequest(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "http", Capability: "write", Decision: "review"},
	}
	proxy, _ := setupProxy(t, rules, "block", nil)

	req := httptest.NewRequest("POST", "/stripe/v1/charges", strings.NewReader(`{"amount":1000}`))
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "pending_review" {
		t.Errorf("expected pending_review status, got %s", body["status"])
	}
	if body["approval_id"] == "" {
		t.Error("expected non-empty approval_id")
	}
}

func TestProxyForwardsToUpstream(t *testing.T) {
	var receivedMethod, receivedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		w.Header().Set("X-Custom", "upstream-header")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"from": "upstream"})
	}))
	defer upstream.Close()

	rules := []toolpolicy.ToolRule{
		{Protocol: "http", Decision: "allow"},
	}
	proxy, _ := setupProxy(t, rules, "allow", upstream)

	req := httptest.NewRequest("POST", "/stripe/v1/charges", strings.NewReader(`{"amount":500}`))
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if receivedMethod != "POST" {
		t.Errorf("upstream received method %s, expected POST", receivedMethod)
	}
	if receivedPath != "/v1/charges" {
		t.Errorf("upstream received path %s, expected /v1/charges", receivedPath)
	}
	if rec.Header().Get("X-Custom") != "upstream-header" {
		t.Error("expected upstream response headers to be forwarded")
	}
}

func TestProxyReturns403OnBlock(t *testing.T) {
	// Default decision is block, no rules => everything blocked.
	proxy, _ := setupProxy(t, nil, "block", nil)

	req := httptest.NewRequest("POST", "/stripe/v1/charges", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if !strings.Contains(body["error"], "blocked") {
		t.Errorf("expected blocked error, got: %s", body["error"])
	}
}

func TestProxyReturns202OnReview(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "http", Decision: "review"},
	}
	proxy, _ := setupProxy(t, rules, "block", nil)

	req := httptest.NewRequest("PUT", "/slack/channels.rename", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rec.Code)
	}
}

func TestEvidenceRecorded(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	engine := toolpolicy.NewEngine([]toolpolicy.ToolRule{
		{Protocol: "http", Decision: "allow"},
	}, "block")
	chain := evidence.NewSessionChain("evidence-test")
	queue := approval.NewQueue(100)
	services := []ServiceConfig{
		{Name: "stripe", UpstreamURL: upstream.URL, PathPrefix: "/stripe"},
	}
	proxy := NewProxy(engine, chain, queue, services)

	req := httptest.NewRequest("GET", "/stripe/v1/balance", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if chain.Count() != 1 {
		t.Errorf("expected 1 evidence record, got %d", chain.Count())
	}

	records := chain.Records()
	if records[0].Envelope.Tool != "stripe.get_v1" {
		t.Errorf("expected tool stripe.get_v1, got %s", records[0].Envelope.Tool)
	}
	if records[0].Envelope.PolicyDecision != "allow" {
		t.Errorf("expected allow decision, got %s", records[0].Envelope.PolicyDecision)
	}
}

func TestUnknownServiceReturns404(t *testing.T) {
	proxy, _ := setupProxy(t, nil, "allow", nil)

	req := httptest.NewRequest("GET", "/unknown/resource", nil)
	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestMethodToCapabilityMapping(t *testing.T) {
	tests := []struct {
		method string
		want   string
	}{
		{"GET", "read"},
		{"HEAD", "read"},
		{"OPTIONS", "read"},
		{"POST", "write"},
		{"PUT", "write"},
		{"PATCH", "write"},
		{"DELETE", "delete"},
		{"CONNECT", "execute"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			got := methodToCapability(tt.method)
			if string(got) != tt.want {
				t.Errorf("methodToCapability(%s) = %s, want %s", tt.method, got, tt.want)
			}
		})
	}
}
