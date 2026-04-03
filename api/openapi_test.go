package api

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPISpecIncludesCurrentRoutes(t *testing.T) {
	data, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}

	var spec struct {
		Paths map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}

	expectedPaths := []string{
		"/",
		"/dashboard",
		"/health",
		"/metrics",
		"/v1/chat/completions",
		"/v1/models",
		"/admin/v1/usage",
		"/admin/v1/providers",
		"/admin/v1/tenants",
		"/admin/v1/policies",
		"/admin/v1/requests",
		"/admin/v1/violations",
		"/admin/v1/cache",
		"/admin/v1/analytics",
		"/admin/v1/analytics/realtime",
		"/admin/v1/alerts",
		"/admin/v1/alerts/{id}/acknowledge",
		"/admin/v1/budgets",
		"/admin/v1/rollouts",
		"/admin/v1/rollouts/{id}",
		"/admin/v1/rollouts/{id}/pause",
		"/admin/v1/rollouts/{id}/resume",
		"/admin/v1/rollouts/{id}/rollback",
		"/admin/v1/audit",
		"/admin/v1/audit/verify",
		"/admin/v1/whoami",
		"/admin/v1/federation/config",
		"/admin/v1/federation/metrics",
		"/admin/v1/federation/status",
		"/admin/v1/federation/planes",
	}

	for _, path := range expectedPaths {
		if _, ok := spec.Paths[path]; !ok {
			t.Fatalf("missing path %s from api/openapi.yaml", path)
		}
	}
}
