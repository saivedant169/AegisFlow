package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/admin"
	"github.com/saivedant169/AegisFlow/internal/behavioral"
	"github.com/saivedant169/AegisFlow/internal/cache"
	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/policy"
	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/router"
	"github.com/saivedant169/AegisFlow/internal/usage"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

// postMessagesAsTenant drives /v1/messages with a tenant in context so the
// governance stages keyed on tenant/session fire.
func postMessagesAsTenant(h *Handler, tenantID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.TenantContextKey, &config.TenantConfig{ID: tenantID})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	h.Messages(w, req)
	return w
}

func newMessagesHandler(recordSpend func(string, string, float64)) (*Handler, *usage.Tracker) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"}}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())
	return NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, recordSpend, nil), ut
}

const msgBody = `{"model":"mock","max_tokens":64,"messages":[{"role":"user","content":"Hello"}]}`

// The /v1/messages path must record spend to the same ledger budgetCheck reads.
// Before the lifecycle unification this never fired (the governance bypass).
func TestMessages_RecordsSpend(t *testing.T) {
	var calls int
	var gotModel string
	var gotCost float64
	h, _ := newMessagesHandler(func(tenantID, model string, cost float64) {
		calls++
		gotModel = model
		gotCost = cost
	})

	if w := postMessagesAsTenant(h, "t1", msgBody); w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if calls != 1 {
		t.Fatalf("expected recordSpend called once on /v1/messages, got %d", calls)
	}
	if gotModel != "mock" {
		t.Errorf("spend model = %q, want mock", gotModel)
	}
	if gotCost <= 0 {
		t.Errorf("spend cost = %v, want > 0", gotCost)
	}
}

// The /v1/messages path must feed behavioral analysis so anomaly detection and
// the kill-switch can see Anthropic traffic.
func TestMessages_RecordsBehavioral(t *testing.T) {
	h, _ := newMessagesHandler(nil)
	reg := behavioral.NewRegistry(behavioral.DefaultRules(), 0, 0)
	h.SetBehavioralRegistry(reg)

	if w := postMessagesAsTenant(h, "t1", msgBody); w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	sa := reg.Get("t1")
	if sa == nil {
		t.Fatal("expected a behavioral analyzer for the tenant session after /v1/messages")
	}
	if got := len(sa.History()); got != 1 {
		t.Fatalf("expected 1 recorded action, got %d", got)
	}
}

// A tenant over its per-model budget must be rejected on /v1/messages, not
// silently served and billed. Before unification this never fired.
func TestMessages_BudgetExceeded(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"}}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())
	budgetCheck := func(tenantID, model string) (bool, []string, string) {
		return false, nil, "budget exhausted"
	}
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, budgetCheck)

	w := postMessagesAsTenant(h, "t1", msgBody)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on over-budget /v1/messages, got %d: %s", w.Code, w.Body.String())
	}
}

// A behavioral-kill-switched session must be blocked on /v1/messages too.
func TestMessages_KillSwitchBlocks(t *testing.T) {
	h, _ := newMessagesHandler(nil)
	reg := behavioral.NewRegistry(behavioral.DefaultRules(), 20, 0) // kill-switch at risk 20
	// Pre-block the tenant's session: three consecutive deletes => destructive
	// sequence (risk 25) >= threshold.
	sa := reg.GetOrCreate("t1")
	for i := 0; i < 3; i++ {
		sa.RecordAction(&envelope.ActionEnvelope{
			Timestamp:           time.Now().UTC(),
			RequestedCapability: envelope.CapDelete,
			Tool:                "shell.rm",
			Target:              "/data",
			Protocol:            envelope.ProtocolMCP,
			Actor:               envelope.ActorInfo{SessionID: "t1"},
		})
	}
	sa.Analyze()
	if !sa.Blocked() {
		t.Fatal("precondition: session should be kill-switch blocked")
	}
	h.SetBehavioralRegistry(reg)

	w := postMessagesAsTenant(h, "t1", msgBody)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a kill-switched session on /v1/messages, got %d: %s", w.Code, w.Body.String())
	}
}

const msgStreamBody = `{"model":"mock","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"Hello"}]}`

// A streaming /v1/messages request must run the post-stream governance tail —
// spend and behavioral previously fired on neither stream path.
func TestMessages_StreamRecordsSpendAndBehavioral(t *testing.T) {
	var spendCalls int
	h, _ := newMessagesHandler(func(tenantID, model string, cost float64) { spendCalls++ })
	reg := behavioral.NewRegistry(behavioral.DefaultRules(), 0, 0)
	h.SetBehavioralRegistry(reg)

	w := postMessagesAsTenant(h, "t1", msgStreamBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if spendCalls != 1 {
		t.Fatalf("expected recordSpend once after a streaming /v1/messages, got %d", spendCalls)
	}
	sa := reg.Get("t1")
	if sa == nil || len(sa.History()) != 1 {
		t.Fatalf("expected one behavioral action recorded after streaming, got %v", sa)
	}
}

// A cached response must be served on /v1/messages, framed as an Anthropic
// envelope, without hitting the provider. Before unification the path never
// consulted the cache.
func TestMessages_CacheHit(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"}}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())
	c := cache.NewMemoryCache(5*time.Minute, 100)
	h := NewHandler(registry, rt, pe, ut, c, nil, nil, nil, 0, nil, nil)

	// Seed the cache under the exact key the Messages handler will compute.
	var in anthropicMessagesRequest
	if err := json.Unmarshal([]byte(msgBody), &in); err != nil {
		t.Fatalf("seed unmarshal: %v", err)
	}
	tr := translateMessagesRequest(&in)
	key := cache.BuildKey("t1", tr.Model, tr.Messages)
	c.Set(key, &types.ChatCompletionResponse{
		Model:   "mock",
		Choices: []types.Choice{{Message: types.Message{Role: "assistant", Content: "CACHED-SENTINEL"}, FinishReason: "stop"}},
		Usage:   types.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	})

	w := postMessagesAsTenant(h, "t1", msgBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp anthropicMessagesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Content) == 0 || resp.Content[0].Text != "CACHED-SENTINEL" {
		t.Fatalf("expected the cached response served as an Anthropic message, got %+v", resp.Content)
	}
}

// An output-policy block on /v1/messages must be written to the admin request
// feed, like the OpenAI path. Before unification the Anthropic path blocked but
// logged nothing (the "partial" divergence).
func TestMessages_OutputBlockLogsRequest(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"}}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, []policy.Filter{policy.NewKeywordFilter("out", policy.ActionBlock, []string{"mock response"})})
	ut := usage.NewTracker(usage.NewStore())
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, nil)
	reqLog := admin.NewRequestLog(10)
	h.SetRequestLogger(reqLog, "test")

	w := postMessagesAsTenant(h, "t1", msgBody)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 output block, got %d: %s", w.Code, w.Body.String())
	}
	entries := reqLog.Recent(1)
	if len(entries) != 1 || entries[0].Status != http.StatusForbidden {
		t.Fatalf("expected a 403 request-log entry on output block, got %+v", entries)
	}
}

// Usage accounting already fired on /v1/messages; keep it as a parity guard so a
// future refactor can't silently drop it.
func TestMessages_RecordsUsage(t *testing.T) {
	h, ut := newMessagesHandler(nil)
	if w := postMessagesAsTenant(h, "t1", msgBody); w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if u := ut.GetUsage("t1"); u == nil || u.TotalTokens == 0 {
		t.Fatalf("expected usage recorded for tenant t1, got %+v", u)
	}
}
