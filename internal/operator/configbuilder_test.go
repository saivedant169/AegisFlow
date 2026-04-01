package operator

import (
	"testing"
	"time"
)

func TestBuildConfigProviders(t *testing.T) {
	cfg := BuildConfig(
		GatewayInput{Port: 8080, AdminPort: 8081, LogLevel: "info", LogFormat: "json"},
		[]ProviderInput{
			{Name: "openai", Type: "openai", BaseURL: "https://api.openai.com/v1", APIKeyEnv: "OPENAI_KEY", Models: []string{"gpt-4o"}, Timeout: 60 * time.Second, Region: "us"},
		},
		[]RouteInput{
			{Model: "gpt-*", Providers: []string{"openai"}, Strategy: "priority"},
		},
		[]TenantInput{
			{ID: "default", Name: "Default", APIKeys: []string{"key-1"}, RequestsPerMinute: 60, TokensPerMinute: 100000, AllowedModels: []string{"*"}},
		},
		[]PolicyInput{
			{Name: "jailbreak", Phase: "input", Type: "keyword", Action: "block", Keywords: []string{"ignore previous"}},
		},
	)

	if len(cfg.Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(cfg.Providers))
	}
	if cfg.Providers[0].Region != "us" {
		t.Errorf("expected region us, got %s", cfg.Providers[0].Region)
	}
	if cfg.Providers[0].Enabled != true {
		t.Error("expected provider to be enabled")
	}
	if len(cfg.Routes) != 1 {
		t.Errorf("expected 1 route, got %d", len(cfg.Routes))
	}
	if cfg.Routes[0].Match.Model != "gpt-*" {
		t.Errorf("expected route model gpt-*, got %s", cfg.Routes[0].Match.Model)
	}
	if len(cfg.Tenants) != 1 {
		t.Errorf("expected 1 tenant, got %d", len(cfg.Tenants))
	}
	if cfg.Tenants[0].RateLimit.RequestsPerMinute != 60 {
		t.Errorf("expected 60 rpm, got %d", cfg.Tenants[0].RateLimit.RequestsPerMinute)
	}
	if len(cfg.Policies.Input) != 1 {
		t.Errorf("expected 1 input policy, got %d", len(cfg.Policies.Input))
	}
	if len(cfg.Policies.Output) != 0 {
		t.Errorf("expected 0 output policies, got %d", len(cfg.Policies.Output))
	}
}

func TestBuildConfigRegions(t *testing.T) {
	cfg := BuildConfig(
		GatewayInput{Port: 8080, AdminPort: 8081},
		nil,
		[]RouteInput{
			{Model: "gpt-4o", Regions: []RegionInput{
				{Name: "us", Providers: []string{"openai-us"}, Strategy: "round-robin"},
				{Name: "eu", Providers: []string{"azure-eu"}, Strategy: "priority"},
			}},
		},
		nil, nil,
	)

	if len(cfg.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(cfg.Routes))
	}
	if len(cfg.Routes[0].Regions) != 2 {
		t.Errorf("expected 2 regions, got %d", len(cfg.Routes[0].Regions))
	}
	if cfg.Routes[0].Regions[0].Name != "us" {
		t.Errorf("expected first region us, got %s", cfg.Routes[0].Regions[0].Name)
	}
	if cfg.Routes[0].Regions[1].Strategy != "priority" {
		t.Errorf("expected second region strategy priority, got %s", cfg.Routes[0].Regions[1].Strategy)
	}
}

func TestBuildConfigOutputPolicy(t *testing.T) {
	cfg := BuildConfig(
		GatewayInput{Port: 8080, AdminPort: 8081},
		nil, nil, nil,
		[]PolicyInput{
			{Name: "pii-filter", Phase: "output", Type: "regex", Action: "redact", Patterns: []string{`\b\d{3}-\d{2}-\d{4}\b`}},
		},
	)

	if len(cfg.Policies.Input) != 0 {
		t.Errorf("expected 0 input policies, got %d", len(cfg.Policies.Input))
	}
	if len(cfg.Policies.Output) != 1 {
		t.Errorf("expected 1 output policy, got %d", len(cfg.Policies.Output))
	}
	if cfg.Policies.Output[0].Name != "pii-filter" {
		t.Errorf("expected policy name pii-filter, got %s", cfg.Policies.Output[0].Name)
	}
}

func TestBuildConfigServerDefaults(t *testing.T) {
	cfg := BuildConfig(
		GatewayInput{Port: 9090, AdminPort: 9091, LogLevel: "debug", LogFormat: "text"},
		nil, nil, nil, nil,
	)

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.AdminPort != 9091 {
		t.Errorf("expected admin port 9091, got %d", cfg.Server.AdminPort)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "text" {
		t.Errorf("expected log format text, got %s", cfg.Logging.Format)
	}
}

func TestBuildConfigEmpty(t *testing.T) {
	cfg := BuildConfig(GatewayInput{}, nil, nil, nil, nil)

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if len(cfg.Providers) != 0 {
		t.Errorf("expected 0 providers, got %d", len(cfg.Providers))
	}
	if len(cfg.Routes) != 0 {
		t.Errorf("expected 0 routes, got %d", len(cfg.Routes))
	}
	if len(cfg.Tenants) != 0 {
		t.Errorf("expected 0 tenants, got %d", len(cfg.Tenants))
	}
}

func TestBuildConfigWasmPolicy(t *testing.T) {
	cfg := BuildConfig(
		GatewayInput{Port: 8080, AdminPort: 8081},
		nil, nil, nil,
		[]PolicyInput{
			{Name: "custom-wasm", Phase: "input", Type: "wasm", Action: "block", WasmPath: "/plugins/filter.wasm", Timeout: 5 * time.Second, OnError: "allow"},
		},
	)

	if len(cfg.Policies.Input) != 1 {
		t.Fatalf("expected 1 input policy, got %d", len(cfg.Policies.Input))
	}
	p := cfg.Policies.Input[0]
	if p.Path != "/plugins/filter.wasm" {
		t.Errorf("expected wasm path, got %s", p.Path)
	}
	if p.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", p.Timeout)
	}
	if p.OnError != "allow" {
		t.Errorf("expected onError allow, got %s", p.OnError)
	}
}

func TestBuildConfigAPIKeyEnvMapping(t *testing.T) {
	cfg := BuildConfig(
		GatewayInput{Port: 8080, AdminPort: 8081},
		[]ProviderInput{
			{Name: "p1", Type: "openai", APIKeyEnv: "MY_SPECIAL_KEY"},
		},
		nil, nil, nil,
	)

	if cfg.Providers[0].APIKeyEnv != "MY_SPECIAL_KEY" {
		t.Errorf("expected API key env MY_SPECIAL_KEY, got %s", cfg.Providers[0].APIKeyEnv)
	}
}

func TestBuildConfigTenantAllowedModels(t *testing.T) {
	cfg := BuildConfig(
		GatewayInput{Port: 8080, AdminPort: 8081},
		nil, nil,
		[]TenantInput{
			{ID: "t1", Name: "Tenant 1", AllowedModels: []string{"gpt-4o", "claude-sonnet-4-20250514"}},
		},
		nil,
	)

	if len(cfg.Tenants) != 1 {
		t.Fatalf("expected 1 tenant, got %d", len(cfg.Tenants))
	}
	if len(cfg.Tenants[0].AllowedModels) != 2 {
		t.Errorf("expected 2 allowed models, got %d", len(cfg.Tenants[0].AllowedModels))
	}
	if cfg.Tenants[0].AllowedModels[0] != "gpt-4o" {
		t.Errorf("expected gpt-4o, got %s", cfg.Tenants[0].AllowedModels[0])
	}
}

func TestBuildConfigMultiplePoliciesBothPhases(t *testing.T) {
	cfg := BuildConfig(
		GatewayInput{Port: 8080, AdminPort: 8081},
		nil, nil, nil,
		[]PolicyInput{
			{Name: "input1", Phase: "input", Type: "keyword", Action: "block", Keywords: []string{"bad"}},
			{Name: "input2", Phase: "input", Type: "regex", Action: "redact", Patterns: []string{`\d{4}`}},
			{Name: "output1", Phase: "output", Type: "wasm", Action: "filter", WasmPath: "/filter.wasm"},
		},
	)

	if len(cfg.Policies.Input) != 2 {
		t.Errorf("expected 2 input policies, got %d", len(cfg.Policies.Input))
	}
	if len(cfg.Policies.Output) != 1 {
		t.Errorf("expected 1 output policy, got %d", len(cfg.Policies.Output))
	}
}
