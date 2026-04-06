package admin

import (
	"context"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/usage"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

func newGraphQLTestServer() *Server {
	cfg := &config.Config{
		Providers: []config.ProviderConfig{
			{Name: "mock", Type: "mock", Enabled: true},
		},
		Tenants: []config.TenantConfig{
			{
				ID:   "t1",
				Name: "Tenant One",
				APIKeys: []config.APIKeyEntry{
					{Key: "key1", Role: "admin"},
				},
				RateLimit:     config.TenantRateLimit{RequestsPerMinute: 60, TokensPerMinute: 100000},
				AllowedModels: []string{"*"},
			},
		},
		Policies: config.PoliciesConfig{
			Input: []config.PolicyConfig{
				{Name: "block-jailbreak", Type: "keyword", Action: "block", Keywords: []string{"hack"}},
			},
		},
		Routes: []config.RouteConfig{
			{Match: config.RouteMatch{Model: "mock"}, Providers: []string{"mock"}, Strategy: "priority"},
		},
	}

	tracker := usage.NewTracker(usage.NewStore())
	tracker.Record("t1", "mock", "mock", types.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15})

	registry := provider.NewRegistry()
	registry.Register(provider.NewMockProvider("mock", 0))

	return NewServer(
		tracker,
		cfg,
		registry,
		NewRequestLog(10),
		nil,                       // cache
		&stubRolloutManager{},     // rollout
		&stubAnalyticsProvider{},  // analytics
		nil,                       // budget
		&verifyOnlyAuditProvider{}, // audit
		nil,                       // federation
		nil,                       // costopt
		nil,                       // evidence
		nil,                       // approval
	)
}

// stubAnalyticsProvider satisfies AnalyticsProvider for tests.
type stubAnalyticsProvider struct{}

func (s *stubAnalyticsProvider) RealtimeSummary() map[string]interface{} {
	return map[string]interface{}{"requests": 42}
}
func (s *stubAnalyticsProvider) RecentAlerts(limit int) interface{} {
	return []interface{}{map[string]interface{}{"id": "a1", "severity": "warning"}}
}
func (s *stubAnalyticsProvider) AcknowledgeAlert(id string) bool {
	return id == "a1"
}
func (s *stubAnalyticsProvider) Dimensions() []string {
	return []string{"provider", "model"}
}

func TestGraphQLQueryUsage(t *testing.T) {
	srv := newGraphQLTestServer()
	result := srv.ExecuteGraphQL(context.Background(), `{ usage { raw } }`, nil)
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", result.Data)
	}
	if _, exists := data["usage"]; !exists {
		t.Fatal("expected 'usage' key in response")
	}
}

func TestGraphQLQueryProviders(t *testing.T) {
	srv := newGraphQLTestServer()
	result := srv.ExecuteGraphQL(context.Background(), `{ providers }`, nil)
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	data := result.Data.(map[string]interface{})
	providers, ok := data["providers"].([]interface{})
	if !ok {
		t.Fatalf("expected providers to be a list, got %T", data["providers"])
	}
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}
}

func TestGraphQLQueryWithArguments(t *testing.T) {
	srv := newGraphQLTestServer()

	// Test usage with tenantId argument
	result := srv.ExecuteGraphQL(context.Background(),
		`{ usage(tenantId: "t1") { raw } }`, nil)
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	// Test alerts with limit argument
	result = srv.ExecuteGraphQL(context.Background(),
		`{ alerts(limit: 5) }`, nil)
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
}

func TestGraphQLMutationAcknowledgeAlert(t *testing.T) {
	srv := newGraphQLTestServer()
	result := srv.ExecuteGraphQL(context.Background(),
		`mutation { acknowledgeAlert(id: "a1") }`, nil)
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	data := result.Data.(map[string]interface{})
	ack, ok := data["acknowledgeAlert"].(bool)
	if !ok || !ack {
		t.Fatalf("expected acknowledgeAlert to return true, got %v", data["acknowledgeAlert"])
	}
}

func TestGraphQLSchemaValidation(t *testing.T) {
	srv := newGraphQLTestServer()

	// Query a field that does not exist
	result := srv.ExecuteGraphQL(context.Background(),
		`{ nonExistentField }`, nil)
	if len(result.Errors) == 0 {
		t.Fatal("expected error for invalid field, got none")
	}
}

func TestGraphQLQueryTenantsPolicies(t *testing.T) {
	srv := newGraphQLTestServer()

	result := srv.ExecuteGraphQL(context.Background(), `{ tenants }`, nil)
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	data := result.Data.(map[string]interface{})
	tenants := data["tenants"].([]interface{})
	if len(tenants) != 1 {
		t.Fatalf("expected 1 tenant, got %d", len(tenants))
	}

	result = srv.ExecuteGraphQL(context.Background(), `{ policies }`, nil)
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	data = result.Data.(map[string]interface{})
	policies := data["policies"].([]interface{})
	if len(policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(policies))
	}
}

func TestGraphQLMutationCreateRollout(t *testing.T) {
	srv := newGraphQLTestServer()
	query := `mutation {
		createRollout(input: {
			routeModel: "mock"
			canaryProvider: "mock"
			stages: [10, 50, 100]
			observationWindow: "1m"
			errorThreshold: 1.5
			latencyP95Threshold: 1000
		})
	}`
	result := srv.ExecuteGraphQL(context.Background(), query, nil)
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
}
