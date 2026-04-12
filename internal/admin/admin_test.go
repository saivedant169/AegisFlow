package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/usage"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

type stubRolloutManager struct{}

func (s *stubRolloutManager) ListRollouts() (any, error) { return []any{}, nil }
func (s *stubRolloutManager) CreateRollout(routeModel string, baselineProviders []string, canaryProvider string, stages []int, observationWindow time.Duration, errorThreshold float64, latencyP95Threshold int64) (any, error) {
	return map[string]any{
		"route_model":        routeModel,
		"baseline_providers": baselineProviders,
		"canary_provider":    canaryProvider,
		"stages":             stages,
	}, nil
}
func (s *stubRolloutManager) GetRolloutWithMetrics(id string) (any, error) {
	return map[string]any{"id": id}, nil
}
func (s *stubRolloutManager) PauseRollout(id string) error    { return nil }
func (s *stubRolloutManager) ResumeRollout(id string) error   { return nil }
func (s *stubRolloutManager) RollbackRollout(id string) error { return nil }

type verifyOnlyAuditProvider struct{}

func (s *verifyOnlyAuditProvider) Query(actor, actorRole, action, tenantID string, limit int) (interface{}, error) {
	return []any{}, nil
}

func (s *verifyOnlyAuditProvider) Verify() (interface{}, error) {
	return map[string]any{"valid": true, "message": "ok"}, nil
}

func (s *verifyOnlyAuditProvider) Log(actor, actorRole, action, resource, detail, tenantID, model string) {
}

func newFullAdminServer() *Server {
	srv := newIntegrationAdminServer()
	srv.approvalProvider = &stubApprovalProvider{
		pendingItems: []map[string]interface{}{
			{"id": "appr-1", "status": "pending", "tool": "github.push"},
		},
		historyItems: []map[string]interface{}{
			{"id": "appr-0", "status": "approved", "reviewer": "alice"},
		},
	}
	srv.evidenceProvider = &stubEvidenceProvider{}
	return srv
}

func newIntegrationAdminServer() *Server {
	cfg := &config.Config{
		Tenants: []config.TenantConfig{
			{ID: "viewer-tenant", Name: "Viewer", APIKeys: []config.APIKeyEntry{{Key: "viewer-key", Role: "viewer"}}},
			{ID: "operator-tenant", Name: "Operator", APIKeys: []config.APIKeyEntry{{Key: "operator-key", Role: "operator"}}},
			{ID: "admin-tenant", Name: "Admin", APIKeys: []config.APIKeyEntry{{Key: "admin-key", Role: "admin"}}},
		},
		Routes: []config.RouteConfig{
			{Match: config.RouteMatch{Model: "mock"}, Providers: []string{"mock"}, Strategy: "priority"},
		},
	}

	tracker := usage.NewTracker(usage.NewStore())
	tracker.Record("viewer-tenant", "mock", "mock", types.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5})
	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))

	return NewServer(
		tracker,
		cfg,
		registry,
		NewRequestLog(10),
		nil,
		&stubRolloutManager{},
		nil,
		nil,
		&verifyOnlyAuditProvider{},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
}

// --- Stub providers for testing ---

type stubApprovalProvider struct {
	pendingItems  []map[string]interface{}
	historyItems  []map[string]interface{}
	approveErr    error
	denyErr       error
}

func (s *stubApprovalProvider) Pending() interface{} { return s.pendingItems }
func (s *stubApprovalProvider) History(limit int) interface{} { return s.historyItems }
func (s *stubApprovalProvider) Get(id string) (interface{}, error) {
	for _, item := range s.pendingItems {
		if item["id"] == id {
			return item, nil
		}
	}
	for _, item := range s.historyItems {
		if item["id"] == id {
			return item, nil
		}
	}
	return nil, errors.New("not found")
}
func (s *stubApprovalProvider) Approve(id, reviewer, comment string) (interface{}, error) {
	if s.approveErr != nil {
		return nil, s.approveErr
	}
	return map[string]interface{}{"id": id, "status": "approved", "reviewer": reviewer}, nil
}
func (s *stubApprovalProvider) Deny(id, reviewer, comment string) (interface{}, error) {
	if s.denyErr != nil {
		return nil, s.denyErr
	}
	return map[string]interface{}{"id": id, "status": "denied", "reviewer": reviewer}, nil
}
func (s *stubApprovalProvider) Submit(env interface{}) (string, error) { return "sub-1", nil }

type stubEvidenceProvider struct{}

func (s *stubEvidenceProvider) ExportSession(id string) (interface{}, error) {
	if id == "missing" {
		return nil, errors.New("session not found")
	}
	return map[string]interface{}{"session_id": id, "records": []interface{}{}}, nil
}
func (s *stubEvidenceProvider) VerifySession(id string) (interface{}, error) {
	return map[string]interface{}{"valid": true, "total_records": 5}, nil
}
func (s *stubEvidenceProvider) ListSessions() interface{} {
	return []map[string]interface{}{
		{"session_id": "sess-1", "total_actions": 3},
	}
}

// stubToolPolicyProvider is a simple ToolPolicyProvider for testing.
type stubToolPolicyProvider struct {
	decision string
}

func (s *stubToolPolicyProvider) Evaluate(env *envelope.ActionEnvelope) string {
	return s.decision
}

func (s *stubToolPolicyProvider) EvaluateWithTrace(env *envelope.ActionEnvelope) interface{} {
	return nil
}

func TestHandleTestAction_Allow(t *testing.T) {
	server := newIntegrationAdminServer()
	server.toolPolicyProvider = &stubToolPolicyProvider{decision: "allow"}
	router := server.Router()

	payload := []byte(`{"protocol":"mcp","tool":"list_repos","target":"github.com/org","capability":"read"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/test-action", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["decision"] != "allow" {
		t.Fatalf("expected allow decision, got %v", body["decision"])
	}
	if body["envelope_id"] == nil || body["envelope_id"] == "" {
		t.Fatal("expected non-empty envelope_id")
	}
	if body["evidence_hash"] == nil || body["evidence_hash"] == "" {
		t.Fatal("expected non-empty evidence_hash")
	}
}

func TestHandleTestAction_Block(t *testing.T) {
	server := newIntegrationAdminServer()
	server.toolPolicyProvider = &stubToolPolicyProvider{decision: "block"}
	router := server.Router()

	payload := []byte(`{"protocol":"shell","tool":"rm","target":"/etc","capability":"delete"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/test-action", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["decision"] != "block" {
		t.Fatalf("expected block decision, got %v", body["decision"])
	}
}

func TestHandleTestAction_Review(t *testing.T) {
	server := newIntegrationAdminServer()
	server.toolPolicyProvider = &stubToolPolicyProvider{decision: "review"}
	router := server.Router()

	payload := []byte(`{"protocol":"git","tool":"push","target":"main","capability":"deploy"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/test-action", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["decision"] != "review" {
		t.Fatalf("expected review decision, got %v", body["decision"])
	}
	if body["message"] != "Action requires human review" {
		t.Fatalf("expected review message, got %v", body["message"])
	}
}

func TestHandleTestAction_MissingFields(t *testing.T) {
	server := newIntegrationAdminServer()
	router := server.Router()

	payload := []byte(`{"protocol":"mcp"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/test-action", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAdminRBACIntegration(t *testing.T) {
	server := newIntegrationAdminServer()
	router := server.Router()

	t.Run("viewer can get usage", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/v1/usage", nil)
		req.Header.Set("X-API-Key", "viewer-key")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		var body map[string]any
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["viewer-tenant"]; !ok {
			t.Fatalf("expected viewer tenant in usage response, got %+v", body)
		}
	})

	t.Run("viewer gets 403 on rollout create", func(t *testing.T) {
		payload := []byte(`{"route_model":"mock","canary_provider":"mock","stages":[10,50,100],"observation_window":"1m","error_threshold":1.5,"latency_p95_threshold":1000}`)
		req := httptest.NewRequest(http.MethodPost, "/admin/v1/rollouts", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", "viewer-key")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", w.Code)
		}
	})

	t.Run("operator can create rollout", func(t *testing.T) {
		payload := []byte(`{"route_model":"mock","canary_provider":"mock","stages":[10,50,100],"observation_window":"1m","error_threshold":1.5,"latency_p95_threshold":1000}`)
		req := httptest.NewRequest(http.MethodPost, "/admin/v1/rollouts", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", "operator-key")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("admin can verify audit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin/v1/audit/verify", nil)
		req.Header.Set("X-API-Key", "admin-key")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})
}

// --- Real-logic handler tests ---

func TestHealthEndpoint(t *testing.T) {
	server := newIntegrationAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %v", body["status"])
	}
}

func TestProvidersEndpoint(t *testing.T) {
	server := newIntegrationAdminServer()
	// Add a provider to cfg so the handler has something to iterate
	server.cfg.Providers = []config.ProviderConfig{
		{Name: "mock", Type: "mock", Enabled: true},
	}
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/providers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if len(body) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(body))
	}
	if body[0]["name"] != "mock" {
		t.Fatalf("expected provider name 'mock', got %v", body[0]["name"])
	}
}

func TestTenantsEndpoint(t *testing.T) {
	server := newIntegrationAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/tenants", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if len(body) != 3 {
		t.Fatalf("expected 3 tenants, got %d", len(body))
	}
}

func TestPoliciesEndpoint(t *testing.T) {
	server := newIntegrationAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/policies", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestViolationsEndpoint(t *testing.T) {
	server := newIntegrationAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/violations", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestCacheEndpointNoCache(t *testing.T) {
	server := newIntegrationAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/cache", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuditQueryEndpoint(t *testing.T) {
	server := newIntegrationAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/audit?limit=10", nil)
	req.Header.Set("X-API-Key", "operator-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSimulateEndpoint(t *testing.T) {
	server := newIntegrationAdminServer()
	server.toolPolicyProvider = &stubToolPolicyProvider{decision: "allow"}
	router := server.Router()

	payload := []byte(`{"protocol":"mcp","tool":"list_repos","target":"github.com/org","capability":"read"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/simulate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	json.NewDecoder(w.Body).Decode(&body)
	if body["decision"] != "allow" {
		t.Fatalf("expected allow, got %v", body["decision"])
	}
}

func TestSimulateEndpointMissingFields(t *testing.T) {
	server := newIntegrationAdminServer()
	server.toolPolicyProvider = &stubToolPolicyProvider{decision: "allow"}
	router := server.Router()

	payload := []byte(`{"protocol":"mcp"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/simulate", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// --- Approval delegation handler tests ---

func TestApprovalsPendingEndpoint(t *testing.T) {
	server := newFullAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/approvals", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestApprovalsHistoryEndpoint(t *testing.T) {
	server := newFullAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/approvals/history", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestApprovalsGetEndpoint(t *testing.T) {
	server := newFullAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/approvals/appr-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestApprovalsGetNotFound(t *testing.T) {
	server := newFullAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/approvals/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestApprovalApproveEndpoint(t *testing.T) {
	server := newFullAdminServer()
	router := server.Router()

	payload := []byte(`{"reviewer":"bob","comment":"looks good"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/approvals/appr-1/approve", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "operator-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestApprovalDenyEndpoint(t *testing.T) {
	server := newFullAdminServer()
	router := server.Router()

	payload := []byte(`{"reviewer":"bob","comment":"too risky"}`)
	req := httptest.NewRequest(http.MethodPost, "/admin/v1/approvals/appr-1/deny", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "operator-key")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// --- Evidence delegation handler tests ---

func TestEvidenceSessionsEndpoint(t *testing.T) {
	server := newFullAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/evidence/sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestEvidenceExportEndpoint(t *testing.T) {
	server := newFullAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/evidence/sessions/sess-1/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestEvidenceExportNotFound(t *testing.T) {
	server := newFullAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/evidence/sessions/missing/export", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestEvidenceVerifyEndpoint(t *testing.T) {
	server := newFullAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodPost, "/admin/v1/evidence/sessions/sess-1/verify", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRolloutsListEndpoint(t *testing.T) {
	server := newIntegrationAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/rollouts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRolloutGetEndpoint(t *testing.T) {
	server := newIntegrationAdminServer()
	router := server.Router()

	req := httptest.NewRequest(http.MethodGet, "/admin/v1/rollouts/roll-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
