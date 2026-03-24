package config

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	yaml := `
server:
  port: 9090
  admin_port: 9091
providers:
  - name: "mock"
    type: "mock"
    enabled: true
    default: true
routes:
  - match:
      model: "*"
    providers: ["mock"]
    strategy: "priority"
tenants:
  - id: "test"
    name: "Test Tenant"
    api_keys:
      - "test-key"
    rate_limit:
      requests_per_minute: 10
      tokens_per_minute: 1000
    allowed_models:
      - "*"
`
	f, err := os.CreateTemp("", "aegisflow-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString(yaml); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.AdminPort != 9091 {
		t.Errorf("expected admin port 9091, got %d", cfg.Server.AdminPort)
	}
	if len(cfg.Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(cfg.Providers))
	}
	if cfg.Providers[0].Name != "mock" {
		t.Errorf("expected provider name 'mock', got '%s'", cfg.Providers[0].Name)
	}
	if len(cfg.Tenants) != 1 {
		t.Errorf("expected 1 tenant, got %d", len(cfg.Tenants))
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host '0.0.0.0', got '%s'", cfg.Server.Host)
	}
}

func TestFindTenantByAPIKey(t *testing.T) {
	cfg := &Config{
		Tenants: []TenantConfig{
			{ID: "t1", APIKeys: []string{"key-a", "key-b"}},
			{ID: "t2", APIKeys: []string{"key-c"}},
		},
	}

	tenant := cfg.FindTenantByAPIKey("key-b")
	if tenant == nil || tenant.ID != "t1" {
		t.Errorf("expected tenant t1, got %v", tenant)
	}

	tenant = cfg.FindTenantByAPIKey("key-c")
	if tenant == nil || tenant.ID != "t2" {
		t.Errorf("expected tenant t2, got %v", tenant)
	}

	tenant = cfg.FindTenantByAPIKey("nonexistent")
	if tenant != nil {
		t.Errorf("expected nil for nonexistent key, got %v", tenant)
	}
}
