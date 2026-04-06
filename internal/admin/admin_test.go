package admin

import (
	"bytes"
	"encoding/json"
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

// stubToolPolicyProvider is a simple ToolPolicyProvider for testing.
type stubToolPolicyProvider struct {
	decision string
}

func (s *stubToolPolicyProvider) Evaluate(env *envelope.ActionEnvelope) string {
	return s.decision
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
