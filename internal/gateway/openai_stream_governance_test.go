package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/behavioral"
	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/policy"
	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/router"
	"github.com/saivedant169/AegisFlow/internal/usage"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

// A streaming /v1/chat/completions request must run the post-stream governance
// tail too — spend and behavioral previously fired on neither stream path.
func TestChatCompletionStream_RecordsSpendAndBehavioral(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"}}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())

	var spendCalls int
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, func(tenantID, model string, cost float64) { spendCalls++ }, nil)
	reg := behavioral.NewRegistry(behavioral.DefaultRules(), 0, 0)
	h.SetBehavioralRegistry(reg)

	body, _ := json.Marshal(types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
		Stream:   true,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.TenantContextKey, &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if spendCalls != 1 {
		t.Fatalf("expected recordSpend once after a streaming completion, got %d", spendCalls)
	}
	if sa := reg.Get("t1"); sa == nil || len(sa.History()) != 1 {
		t.Fatalf("expected one behavioral action recorded after streaming, got %v", sa)
	}
}
