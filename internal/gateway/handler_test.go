package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/analytics"
	"github.com/saivedant169/AegisFlow/internal/cache"
	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/eval"
	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/policy"
	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/router"
	"github.com/saivedant169/AegisFlow/internal/storage"
	"github.com/saivedant169/AegisFlow/internal/usage"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

func setupTestHandler() *Handler {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())
	return NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, nil)
}

func TestChatCompletionSuccess(t *testing.T) {
	h := setupTestHandler()

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp types.ChatCompletionResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got '%s'", resp.Choices[0].FinishReason)
	}
}

func TestChatCompletionMissingModel(t *testing.T) {
	h := setupTestHandler()

	reqBody := types.ChatCompletionRequest{
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatCompletionMissingMessages(t *testing.T) {
	h := setupTestHandler()

	reqBody := types.ChatCompletionRequest{Model: "mock"}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatCompletionStream(t *testing.T) {
	h := setupTestHandler()

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
		Stream:   true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", contentType)
	}
}

func TestListModels(t *testing.T) {
	h := setupTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()

	h.ListModels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp types.ModelList
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Object != "list" {
		t.Errorf("expected object 'list', got '%s'", resp.Object)
	}
	if len(resp.Data) == 0 {
		t.Error("expected at least 1 model")
	}
}

func TestInvalidJSON(t *testing.T) {
	h := setupTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatCompletionPolicyBlock(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	// Create a policy engine with a keyword filter that blocks "forbidden"
	inputFilters := []policy.Filter{
		policy.NewKeywordFilter("block-test", policy.ActionBlock, []string{"forbidden"}),
	}
	pe := policy.NewEngine(inputFilters, nil)
	ut := usage.NewTracker(usage.NewStore())
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, nil)

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "This is forbidden content"}},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestChatCompletionBudgetBlock(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())

	budgetCheck := func(tenantID, model string) (bool, []string, string) {
		return false, nil, "budget exhausted"
	}
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, budgetCheck)

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)

	// Need to set a tenant context for budget check to fire
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.TenantContextKey, &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
}

func TestChatCompletionCacheHit(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())
	c := cache.NewMemoryCache(5*time.Minute, 100)
	h := NewHandler(registry, rt, pe, ut, c, nil, nil, nil, 0, nil, nil)

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello cache"}},
	}
	body, _ := json.Marshal(reqBody)

	// First request: cache miss
	req1 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	h.ChatCompletion(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w1.Code)
	}
	if w1.Header().Get("X-AegisFlow-Cache") != "MISS" {
		t.Errorf("first request: expected cache MISS header, got %q", w1.Header().Get("X-AegisFlow-Cache"))
	}

	// Second request with same body: cache hit
	body2, _ := json.Marshal(reqBody)
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	h.ChatCompletion(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second request: expected 200, got %d", w2.Code)
	}
	if w2.Header().Get("X-AegisFlow-Cache") != "HIT" {
		t.Errorf("second request: expected cache HIT header, got %q", w2.Header().Get("X-AegisFlow-Cache"))
	}
}

func TestChatCompletionRecordsAnalytics(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())
	ac := analytics.NewCollector(1)
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, ac, 0, nil, nil)

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "analytics test"}},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// If we got here without panic, analytics recording path was exercised
}

func TestListModelsReturnsModels(t *testing.T) {
	h := setupTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()

	h.ListModels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp types.ModelList
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Object != "list" {
		t.Errorf("expected 'list', got %q", resp.Object)
	}
	// MockProvider returns 2 models (mock, mock-fast)
	if len(resp.Data) < 1 {
		t.Error("expected at least 1 model in response")
	}
}

func TestChatCompletionOutputPolicyBlock(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	// Output filter that blocks mock response content
	outputFilters := []policy.Filter{
		policy.NewKeywordFilter("output-block", policy.ActionBlock, []string{"mock response"}),
	}
	pe := policy.NewEngine(nil, outputFilters)
	ut := usage.NewTracker(usage.NewStore())
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, nil)

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for output policy block, got %d", w.Code)
	}
}

func TestChatCompletionWithSpendRecording(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())

	var recordedTenant, recordedModel string
	var recordedCost float64
	recordSpend := func(tenantID, model string, cost float64) {
		recordedTenant = tenantID
		recordedModel = model
		recordedCost = cost
	}
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, recordSpend, nil)

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// recordSpend should have been called (tenant is empty since no context)
	if recordedModel != "mock" {
		t.Errorf("expected model 'mock', got %q", recordedModel)
	}
	if recordedCost <= 0 {
		t.Error("expected positive cost to be recorded")
	}
	_ = recordedTenant
}

func TestChatCompletionWithEval(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, nil)
	h.SetEval(true, 5, 2.0, nil)

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello eval test"}},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSetAuditLogger(t *testing.T) {
	h := setupTestHandler()
	called := false
	h.SetAuditLogger(func(actor, actorRole, action, resource, detail, tenantID, model string) {
		called = true
	})
	if h.auditLog == nil {
		t.Error("expected auditLog to be set")
	}
	// Trigger it manually
	h.auditLog("a", "b", "c", "d", "e", "f", "g")
	if !called {
		t.Error("expected audit log function to be called")
	}
}

func TestTransformRequestNilConfig(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	// Should not panic with nil config
	TransformRequest(req, nil)
	if req.Messages[0].Content != "Hello" {
		t.Error("expected content unchanged with nil config")
	}
}

func TestTransformRequestSystemPromptPrefix(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Messages: []types.Message{
			{Role: "system", Content: "Be helpful"},
			{Role: "user", Content: "Hello"},
		},
	}
	TransformRequest(req, &TransformConfig{SystemPromptPrefix: "PREFIX"})
	if req.Messages[0].Content != "PREFIX Be helpful" {
		t.Errorf("expected prefixed system prompt, got %q", req.Messages[0].Content)
	}
}

func TestTransformRequestDefaultSystemPrompt(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Messages: []types.Message{
			{Role: "user", Content: "Hello"},
		},
	}
	TransformRequest(req, &TransformConfig{DefaultSystemPrompt: "You are an AI assistant"})
	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "You are an AI assistant" {
		t.Errorf("expected default system prompt, got %q", req.Messages[0].Content)
	}
}

func TestChatCompletionPolicyBlockWithAuditLog(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	inputFilters := []policy.Filter{
		policy.NewKeywordFilter("block-test", policy.ActionBlock, []string{"forbidden"}),
	}
	pe := policy.NewEngine(inputFilters, nil)
	ut := usage.NewTracker(usage.NewStore())
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, nil)

	auditCalled := false
	h.SetAuditLogger(func(actor, actorRole, action, resource, detail, tenantID, model string) {
		auditCalled = true
	})

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "This is forbidden content"}},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.TenantContextKey, &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if !auditCalled {
		t.Error("expected audit logger to be called on policy block")
	}
}

func TestChatCompletionOutputPolicyBlockWithAuditLog(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	outputFilters := []policy.Filter{
		policy.NewKeywordFilter("output-block", policy.ActionBlock, []string{"mock response"}),
	}
	pe := policy.NewEngine(nil, outputFilters)
	ut := usage.NewTracker(usage.NewStore())
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, nil)

	auditCalled := false
	h.SetAuditLogger(func(actor, actorRole, action, resource, detail, tenantID, model string) {
		auditCalled = true
	})

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.TenantContextKey, &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if !auditCalled {
		t.Error("expected audit logger to be called on output policy block")
	}
}

func TestChatCompletionWithEvalWebhook(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, nil)

	// Create a test webhook server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer server.Close()

	evalWH := eval.NewWebhookEvaluator(server.URL, 1.0, 5*time.Second, false)
	h.SetEval(true, 5, 2.0, evalWH)

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello webhook eval"}},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestTransformRequestSystemPromptSuffix(t *testing.T) {
	req := &types.ChatCompletionRequest{
		Messages: []types.Message{
			{Role: "system", Content: "Be helpful"},
			{Role: "user", Content: "Hello"},
		},
	}
	TransformRequest(req, &TransformConfig{SystemPromptSuffix: "SUFFIX"})
	// Note: the suffix logic reads from original msg.Content, not the prefixed version
	if req.Messages[0].Content != "Be helpful SUFFIX" {
		t.Errorf("expected suffixed system prompt, got %q", req.Messages[0].Content)
	}
}

func TestChatCompletionPolicyWarn(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	// Warn action, not block
	inputFilters := []policy.Filter{
		policy.NewKeywordFilter("warn-test", policy.ActionWarn, []string{"suspicious"}),
	}
	pe := policy.NewEngine(inputFilters, nil)
	ut := usage.NewTracker(usage.NewStore())
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, nil)

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "This is suspicious content"}},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	// Warn should allow the request through
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for policy warn, got %d", w.Code)
	}
}

func TestChatCompletionBudgetWarnings(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))
	routes := []config.RouteConfig{
		{Match: config.RouteMatch{Model: "*"}, Providers: []string{"mock"}, Strategy: "priority"},
	}
	rt := router.NewRouter(routes, registry)
	pe := policy.NewEngine(nil, nil)
	ut := usage.NewTracker(usage.NewStore())

	budgetCheck := func(tenantID, model string) (bool, []string, string) {
		return true, []string{"budget 90% used"}, ""
	}
	h := NewHandler(registry, rt, pe, ut, nil, nil, nil, nil, 0, nil, budgetCheck)

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.TenantContextKey, &config.TenantConfig{ID: "t1"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	h.ChatCompletion(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	warning := w.Header().Get("X-AegisFlow-Budget-Warning")
	if warning != "budget 90% used" {
		t.Errorf("expected budget warning header, got %q", warning)
	}
}

func TestClose(t *testing.T) {
	h := setupTestHandler()
	// Close with no dbQueue should not panic
	h.Close()

	// Close with dbQueue
	h2 := setupTestHandler()
	h2.dbQueue = make(chan storage.UsageEvent, 10)
	h2.Close()
	// Verify channel is closed by checking that send would panic (we can check receive returns zero value)
	_, ok := <-h2.dbQueue
	if ok {
		t.Error("expected dbQueue channel to be closed")
	}
}
