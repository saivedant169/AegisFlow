package config

import (
	"os"
	"strings"
	"testing"
)

// loadFromYAML is a test helper that writes YAML to a temp file and calls Load.
func loadFromYAML(t *testing.T, yamlContent string) (*Config, error) {
	t.Helper()
	f, err := os.CreateTemp("", "aegisflow-validate-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(yamlContent); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return Load(f.Name())
}

// expectValidationError asserts that Load fails with a message containing substr.
func expectValidationError(t *testing.T, yamlContent, substr string) {
	t.Helper()
	_, err := loadFromYAML(t, yamlContent)
	if err == nil {
		t.Fatalf("expected validation error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got: %v", substr, err)
	}
}

func TestValidateProviderEmptyName(t *testing.T) {
	expectValidationError(t, `
providers:
  - name: ""
    type: "openai"
    enabled: true
`, "provider name")
}

func TestValidateProviderEmptyType(t *testing.T) {
	expectValidationError(t, `
providers:
  - name: "my-openai"
    type: ""
    enabled: true
`, "provider type")
}

func TestValidateDuplicateProviderNames(t *testing.T) {
	expectValidationError(t, `
providers:
  - name: "openai-1"
    type: "openai"
    enabled: true
  - name: "openai-1"
    type: "openai"
    enabled: true
`, "duplicate provider name")
}

func TestValidateRouteReferencesNonExistentProvider(t *testing.T) {
	expectValidationError(t, `
providers:
  - name: "mock"
    type: "mock"
    enabled: true
routes:
  - match:
      model: "gpt-4"
    providers: ["nonexistent"]
    strategy: "priority"
`, "references unknown provider")
}

func TestValidateRouteEmptyProviders(t *testing.T) {
	expectValidationError(t, `
providers:
  - name: "mock"
    type: "mock"
    enabled: true
routes:
  - match:
      model: "gpt-4"
    providers: []
    strategy: "priority"
`, "no providers")
}

func TestValidateTenantEmptyID(t *testing.T) {
	expectValidationError(t, `
tenants:
  - id: ""
    name: "Bad Tenant"
    api_keys: ["key-1"]
`, "tenant id")
}

func TestValidateTenantNoAPIKeys(t *testing.T) {
	expectValidationError(t, `
tenants:
  - id: "t1"
    name: "No Keys"
    api_keys: []
`, "api_keys")
}

func TestValidateDuplicateTenantIDs(t *testing.T) {
	expectValidationError(t, `
tenants:
  - id: "t1"
    name: "Tenant A"
    api_keys: ["key-a"]
  - id: "t1"
    name: "Tenant B"
    api_keys: ["key-b"]
`, "duplicate tenant id")
}

func TestValidateNegativeRateLimits(t *testing.T) {
	expectValidationError(t, `
tenants:
  - id: "t1"
    name: "Bad Limits"
    api_keys: ["key-a"]
    rate_limit:
      requests_per_minute: -5
      tokens_per_minute: 1000
`, "requests_per_minute")
}

func TestValidateNegativeTokensPerMinute(t *testing.T) {
	expectValidationError(t, `
tenants:
  - id: "t1"
    name: "Bad Limits"
    api_keys: ["key-a"]
    rate_limit:
      requests_per_minute: 100
      tokens_per_minute: -10
`, "tokens_per_minute")
}

func TestValidateNegativeMaxBodySize(t *testing.T) {
	expectValidationError(t, `
server:
  max_body_size: -1024
`, "max_body_size")
}

func TestValidateValidConfigPasses(t *testing.T) {
	cfg, err := loadFromYAML(t, `
server:
  port: 8080
providers:
  - name: "mock"
    type: "mock"
    enabled: true
routes:
  - match:
      model: "*"
    providers: ["mock"]
    strategy: "priority"
tenants:
  - id: "t1"
    name: "Test"
    api_keys: ["test-key"]
    rate_limit:
      requests_per_minute: 60
      tokens_per_minute: 10000
`)
	if err != nil {
		t.Fatalf("expected valid config to load, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
}
