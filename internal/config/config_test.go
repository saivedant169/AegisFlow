package config

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestLoadConfig(t *testing.T) {
	yaml := `
server:
  port: 9090
  admin_port: 9091
compression:
  enabled: true
  min_size_bytes: 256
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
	if !cfg.Compression.Enabled || cfg.Compression.MinSizeBytes != 256 {
		t.Errorf("expected compression config {true,256}, got {%v,%d}", cfg.Compression.Enabled, cfg.Compression.MinSizeBytes)
	}
}

func TestFindTenantByAPIKey(t *testing.T) {
	cfg := &Config{
		Tenants: []TenantConfig{
			{ID: "t1", APIKeys: []APIKeyEntry{{Key: "key-a", Role: "operator"}, {Key: "key-b", Role: "operator"}}},
			{ID: "t2", APIKeys: []APIKeyEntry{{Key: "key-c", Role: "operator"}}},
		},
	}

	match := cfg.FindTenantByAPIKey("key-b")
	if match == nil || match.Tenant.ID != "t1" {
		t.Errorf("expected tenant t1, got %v", match)
	}

	match = cfg.FindTenantByAPIKey("key-c")
	if match == nil || match.Tenant.ID != "t2" {
		t.Errorf("expected tenant t2, got %v", match)
	}

	match = cfg.FindTenantByAPIKey("nonexistent")
	if match != nil {
		t.Errorf("expected nil for nonexistent key, got %v", match)
	}
}

func TestAPIKeyEntryNewFormat(t *testing.T) {
	yamlData := `
tenants:
  - id: "t1"
    name: "Tenant 1"
    api_keys:
      - key: "admin-key"
        role: "admin"
      - key: "viewer-key"
        role: "viewer"
`
	f, err := os.CreateTemp("", "aegisflow-newformat-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yamlData)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if len(cfg.Tenants) != 1 {
		t.Fatalf("expected 1 tenant, got %d", len(cfg.Tenants))
	}
	keys := cfg.Tenants[0].APIKeys
	if len(keys) != 2 {
		t.Fatalf("expected 2 api keys, got %d", len(keys))
	}
	if keys[0].Key != "admin-key" || keys[0].Role != "admin" {
		t.Errorf("expected {admin-key, admin}, got {%s, %s}", keys[0].Key, keys[0].Role)
	}
	if keys[1].Key != "viewer-key" || keys[1].Role != "viewer" {
		t.Errorf("expected {viewer-key, viewer}, got {%s, %s}", keys[1].Key, keys[1].Role)
	}
}

func TestAPIKeyEntryOldFormat(t *testing.T) {
	yamlData := `
tenants:
  - id: "t1"
    name: "Tenant 1"
    api_keys:
      - "plain-key-1"
      - "plain-key-2"
`
	f, err := os.CreateTemp("", "aegisflow-oldformat-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yamlData)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	keys := cfg.Tenants[0].APIKeys
	if len(keys) != 2 {
		t.Fatalf("expected 2 api keys, got %d", len(keys))
	}
	if keys[0].Key != "plain-key-1" || keys[0].Role != "operator" {
		t.Errorf("expected {plain-key-1, operator}, got {%s, %s}", keys[0].Key, keys[0].Role)
	}
	if keys[1].Key != "plain-key-2" || keys[1].Role != "operator" {
		t.Errorf("expected {plain-key-2, operator}, got {%s, %s}", keys[1].Key, keys[1].Role)
	}
}

func TestAPIKeyEntryMixedFormat(t *testing.T) {
	yamlData := `
tenants:
  - id: "t1"
    name: "Tenant 1"
    api_keys:
      - "plain-key"
      - key: "structured-key"
        role: "admin"
`
	f, err := os.CreateTemp("", "aegisflow-mixed-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yamlData)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	keys := cfg.Tenants[0].APIKeys
	if len(keys) != 2 {
		t.Fatalf("expected 2 api keys, got %d", len(keys))
	}
	if keys[0].Key != "plain-key" || keys[0].Role != "operator" {
		t.Errorf("expected {plain-key, operator}, got {%s, %s}", keys[0].Key, keys[0].Role)
	}
	if keys[1].Key != "structured-key" || keys[1].Role != "admin" {
		t.Errorf("expected {structured-key, admin}, got {%s, %s}", keys[1].Key, keys[1].Role)
	}
}

func TestAPIKeyEntryDefaultRole(t *testing.T) {
	yamlData := `
tenants:
  - id: "t1"
    name: "Tenant 1"
    api_keys:
      - key: "no-role-key"
`
	f, err := os.CreateTemp("", "aegisflow-defaultrole-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yamlData)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	keys := cfg.Tenants[0].APIKeys
	if len(keys) != 1 {
		t.Fatalf("expected 1 api key, got %d", len(keys))
	}
	if keys[0].Key != "no-role-key" || keys[0].Role != "operator" {
		t.Errorf("expected {no-role-key, operator}, got {%s, %s}", keys[0].Key, keys[0].Role)
	}
}

func TestFindTenantByAPIKeyReturnsRole(t *testing.T) {
	cfg := &Config{
		Tenants: []TenantConfig{
			{
				ID: "t1",
				APIKeys: []APIKeyEntry{
					{Key: "op-key", Role: "operator"},
					{Key: "admin-key", Role: "admin"},
				},
			},
		},
	}

	match := cfg.FindTenantByAPIKey("op-key")
	if match == nil {
		t.Fatal("expected match, got nil")
	}
	if match.Role != "operator" {
		t.Errorf("expected role operator, got %s", match.Role)
	}

	match = cfg.FindTenantByAPIKey("admin-key")
	if match == nil {
		t.Fatal("expected match, got nil")
	}
	if match.Role != "admin" {
		t.Errorf("expected role admin, got %s", match.Role)
	}
	if match.Tenant.ID != "t1" {
		t.Errorf("expected tenant t1, got %s", match.Tenant.ID)
	}
}

func TestSetDefaultsAllValues(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)

	// Server defaults
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.AdminPort != 8081 {
		t.Errorf("expected admin port 8081, got %d", cfg.Server.AdminPort)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("expected read timeout 30s, got %v", cfg.Server.ReadTimeout)
	}
	if cfg.Server.WriteTimeout != 120*time.Second {
		t.Errorf("expected write timeout 120s, got %v", cfg.Server.WriteTimeout)
	}
	if cfg.Server.GracefulShutdown != 10*time.Second {
		t.Errorf("expected graceful shutdown 10s, got %v", cfg.Server.GracefulShutdown)
	}

	// Rate limit
	if cfg.RateLimit.Backend != "memory" {
		t.Errorf("expected rate limit backend memory, got %s", cfg.RateLimit.Backend)
	}

	// Logging
	if cfg.Logging.Level != "info" {
		t.Errorf("expected logging level info, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected logging format json, got %s", cfg.Logging.Format)
	}

	// Telemetry
	if cfg.Telemetry.Exporter != "stdout" {
		t.Errorf("expected telemetry exporter stdout, got %s", cfg.Telemetry.Exporter)
	}
	if cfg.Telemetry.Metrics.Path != "/metrics" {
		t.Errorf("expected metrics path /metrics, got %s", cfg.Telemetry.Metrics.Path)
	}

	// Cache
	if cfg.Cache.Backend != "memory" {
		t.Errorf("expected cache backend memory, got %s", cfg.Cache.Backend)
	}
	if cfg.Cache.TTL != 5*time.Minute {
		t.Errorf("expected cache TTL 5m, got %v", cfg.Cache.TTL)
	}
	if cfg.Cache.MaxSize != 1000 {
		t.Errorf("expected cache max size 1000, got %d", cfg.Cache.MaxSize)
	}
	if cfg.Compression.MinSizeBytes != 1024 {
		t.Errorf("expected compression min size 1024, got %d", cfg.Compression.MinSizeBytes)
	}

	// Analytics
	if cfg.Analytics.RetentionHours != 48 {
		t.Errorf("expected retention hours 48, got %d", cfg.Analytics.RetentionHours)
	}
	if cfg.Analytics.FlushInterval != time.Hour {
		t.Errorf("expected flush interval 1h, got %v", cfg.Analytics.FlushInterval)
	}
	if cfg.Analytics.AnomalyDetection.EvaluationInterval != time.Minute {
		t.Errorf("expected evaluation interval 1m, got %v", cfg.Analytics.AnomalyDetection.EvaluationInterval)
	}
	if cfg.Analytics.AnomalyDetection.Static.ErrorRateMax != 20 {
		t.Errorf("expected error rate max 20, got %f", cfg.Analytics.AnomalyDetection.Static.ErrorRateMax)
	}
	if cfg.Analytics.AnomalyDetection.Static.P95LatencyMax != 5000 {
		t.Errorf("expected p95 latency max 5000, got %d", cfg.Analytics.AnomalyDetection.Static.P95LatencyMax)
	}
	if cfg.Analytics.AnomalyDetection.Static.RequestsPerMinuteMax != 10000 {
		t.Errorf("expected requests per minute max 10000, got %d", cfg.Analytics.AnomalyDetection.Static.RequestsPerMinuteMax)
	}
	if cfg.Analytics.AnomalyDetection.Static.CostPerMinuteMax != 50.0 {
		t.Errorf("expected cost per minute max 50.0, got %f", cfg.Analytics.AnomalyDetection.Static.CostPerMinuteMax)
	}
	if cfg.Analytics.AnomalyDetection.Baseline.Window != 24*time.Hour {
		t.Errorf("expected baseline window 24h, got %v", cfg.Analytics.AnomalyDetection.Baseline.Window)
	}
	if cfg.Analytics.AnomalyDetection.Baseline.StddevThreshold != 3 {
		t.Errorf("expected stddev threshold 3, got %f", cfg.Analytics.AnomalyDetection.Baseline.StddevThreshold)
	}

	// Budget defaults
	if cfg.Budgets.Global.AlertAt != 80 {
		t.Errorf("expected budget alert at 80, got %d", cfg.Budgets.Global.AlertAt)
	}
	if cfg.Budgets.Global.WarnAt != 90 {
		t.Errorf("expected budget warn at 90, got %d", cfg.Budgets.Global.WarnAt)
	}

	// Eval defaults
	if cfg.Eval.Builtin.MinResponseTokens != 10 {
		t.Errorf("expected min response tokens 10, got %d", cfg.Eval.Builtin.MinResponseTokens)
	}
	if cfg.Eval.Builtin.LatencyMultiplier != 2.0 {
		t.Errorf("expected latency multiplier 2.0, got %f", cfg.Eval.Builtin.LatencyMultiplier)
	}
	if cfg.Eval.Webhook.Timeout != 5*time.Second {
		t.Errorf("expected eval webhook timeout 5s, got %v", cfg.Eval.Webhook.Timeout)
	}

	// Federation defaults
	if cfg.Federation.ControlPlane.SyncInterval != 30*time.Second {
		t.Errorf("expected federation sync interval 30s, got %v", cfg.Federation.ControlPlane.SyncInterval)
	}
}

func TestSetDefaultsCORSEnabled(t *testing.T) {
	cfg := &Config{}
	cfg.Server.CORS.Enabled = true
	setDefaults(cfg)

	if len(cfg.Server.CORS.AllowedOrigins) != 1 || cfg.Server.CORS.AllowedOrigins[0] != "*" {
		t.Errorf("expected CORS allowed origins [*], got %v", cfg.Server.CORS.AllowedOrigins)
	}
	if len(cfg.Server.CORS.AllowedMethods) != 5 {
		t.Errorf("expected 5 CORS allowed methods, got %d", len(cfg.Server.CORS.AllowedMethods))
	}
	if len(cfg.Server.CORS.AllowedHeaders) != 3 {
		t.Errorf("expected 3 CORS allowed headers, got %d", len(cfg.Server.CORS.AllowedHeaders))
	}
	if cfg.Server.CORS.MaxAge != 86400 {
		t.Errorf("expected CORS max age 86400, got %d", cfg.Server.CORS.MaxAge)
	}
}

func TestSetDefaultsCORSDisabledNoOverride(t *testing.T) {
	cfg := &Config{}
	setDefaults(cfg)

	// When CORS is not enabled, CORS-specific defaults should NOT be set
	if len(cfg.Server.CORS.AllowedOrigins) != 0 {
		t.Errorf("expected no CORS origins when disabled, got %v", cfg.Server.CORS.AllowedOrigins)
	}
	if len(cfg.Server.CORS.AllowedMethods) != 0 {
		t.Errorf("expected no CORS methods when disabled, got %v", cfg.Server.CORS.AllowedMethods)
	}
}

func TestSetDefaultsDoesNotOverrideExisting(t *testing.T) {
	cfg := &Config{}
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 3000
	cfg.Logging.Level = "debug"
	cfg.Cache.TTL = 10 * time.Minute
	setDefaults(cfg)

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("expected port 3000, got %d", cfg.Server.Port)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected logging level debug, got %s", cfg.Logging.Level)
	}
	if cfg.Cache.TTL != 10*time.Minute {
		t.Errorf("expected cache TTL 10m, got %v", cfg.Cache.TTL)
	}
}

func TestAPIKeyEntryUnmarshalYAMLInvalidNode(t *testing.T) {
	// Simulate a sequence node (invalid for APIKeyEntry)
	node := &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
	}
	var entry APIKeyEntry
	err := entry.UnmarshalYAML(node)
	if err == nil {
		t.Error("expected error for sequence node, got nil")
	}
}

func TestFindTenantByAPIKeyEmptyKey(t *testing.T) {
	cfg := &Config{
		Tenants: []TenantConfig{
			{ID: "t1", APIKeys: []APIKeyEntry{{Key: "key-a", Role: "operator"}}},
		},
	}
	match := cfg.FindTenantByAPIKey("")
	if match != nil {
		t.Errorf("expected nil for empty key, got %v", match)
	}
}

func TestFindTenantByAPIKeyEmptyTenants(t *testing.T) {
	cfg := &Config{}
	match := cfg.FindTenantByAPIKey("any-key")
	if match != nil {
		t.Errorf("expected nil for empty tenants, got %v", match)
	}
}

func TestFindTenantByAPIKeyTimingSafety(t *testing.T) {
	// Both existing and non-existing keys should iterate all tenants (constant time)
	cfg := &Config{
		Tenants: []TenantConfig{
			{ID: "t1", APIKeys: []APIKeyEntry{{Key: "key-a", Role: "operator"}}},
			{ID: "t2", APIKeys: []APIKeyEntry{{Key: "key-b", Role: "admin"}}},
		},
	}
	// Non-match returns nil
	match := cfg.FindTenantByAPIKey("wrong-key")
	if match != nil {
		t.Errorf("expected nil for wrong key, got %v", match)
	}
	// Match on second tenant
	match = cfg.FindTenantByAPIKey("key-b")
	if match == nil || match.Tenant.ID != "t2" {
		t.Errorf("expected tenant t2, got %v", match)
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	f, err := os.CreateTemp("", "aegisflow-invalid-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("{{{{invalid yaml content")
	f.Close()

	_, err = Load(f.Name())
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestCanaryConfigParsing(t *testing.T) {
	yamlData := `
routes:
  - match:
      model: "gpt-4"
    providers: ["openai", "azure"]
    strategy: "canary"
    canary:
      target_provider: "azure"
      stages: [10, 25, 50, 100]
      observation_window: 5m
      error_threshold: 5.0
      latency_p95_threshold: 3000
`
	f, err := os.CreateTemp("", "aegisflow-canary-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yamlData)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if len(cfg.Routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(cfg.Routes))
	}
	canary := cfg.Routes[0].Canary
	if canary == nil {
		t.Fatal("expected canary config, got nil")
	}
	if canary.TargetProvider != "azure" {
		t.Errorf("expected target provider azure, got %s", canary.TargetProvider)
	}
	if len(canary.Stages) != 4 || canary.Stages[0] != 10 {
		t.Errorf("expected stages [10,25,50,100], got %v", canary.Stages)
	}
	if canary.ObservationWindow != 5*time.Minute {
		t.Errorf("expected observation window 5m, got %v", canary.ObservationWindow)
	}
	if canary.ErrorThreshold != 5.0 {
		t.Errorf("expected error threshold 5.0, got %f", canary.ErrorThreshold)
	}
	if canary.LatencyP95Threshold != 3000 {
		t.Errorf("expected latency p95 threshold 3000, got %d", canary.LatencyP95Threshold)
	}
}

func TestRegionConfigParsing(t *testing.T) {
	yamlData := `
routes:
  - match:
      model: "gpt-4"
    providers: ["openai"]
    strategy: "geo"
    regions:
      - name: "us-east"
        providers: ["openai-us"]
        strategy: "priority"
      - name: "eu-west"
        providers: ["openai-eu"]
        strategy: "round-robin"
`
	f, err := os.CreateTemp("", "aegisflow-region-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yamlData)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	regions := cfg.Routes[0].Regions
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions, got %d", len(regions))
	}
	if regions[0].Name != "us-east" || regions[0].Strategy != "priority" {
		t.Errorf("unexpected region[0]: %+v", regions[0])
	}
	if regions[1].Name != "eu-west" || regions[1].Strategy != "round-robin" {
		t.Errorf("unexpected region[1]: %+v", regions[1])
	}
}

func TestBudgetConfigParsing(t *testing.T) {
	yamlData := `
budgets:
  enabled: true
  global:
    monthly: 1000.0
    daily: 50.0
    alert_at: 75
    warn_at: 90
  tenants:
    tenant1:
      monthly: 500.0
      daily: 25.0
      alert_at: 70
      warn_at: 85
      models:
        gpt-4:
          monthly: 200.0
          daily: 10.0
`
	f, err := os.CreateTemp("", "aegisflow-budget-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yamlData)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if !cfg.Budgets.Enabled {
		t.Error("expected budgets enabled")
	}
	if cfg.Budgets.Global.Monthly != 1000.0 {
		t.Errorf("expected global monthly 1000, got %f", cfg.Budgets.Global.Monthly)
	}
	if cfg.Budgets.Global.Daily != 50.0 {
		t.Errorf("expected global daily 50, got %f", cfg.Budgets.Global.Daily)
	}
	if cfg.Budgets.Global.AlertAt != 75 {
		t.Errorf("expected global alert_at 75, got %d", cfg.Budgets.Global.AlertAt)
	}

	tb, ok := cfg.Budgets.Tenants["tenant1"]
	if !ok {
		t.Fatal("expected tenant1 budget config")
	}
	if tb.Monthly != 500.0 {
		t.Errorf("expected tenant monthly 500, got %f", tb.Monthly)
	}
	if tb.AlertAt != 70 {
		t.Errorf("expected tenant alert_at 70, got %d", tb.AlertAt)
	}
	modelBudget, ok := tb.Models["gpt-4"]
	if !ok {
		t.Fatal("expected gpt-4 model budget")
	}
	if modelBudget.Monthly != 200.0 {
		t.Errorf("expected model monthly 200, got %f", modelBudget.Monthly)
	}
}

func TestEvalConfigParsing(t *testing.T) {
	yamlData := `
eval:
  enabled: true
  builtin:
    enabled: true
    min_response_tokens: 20
    latency_multiplier: 3.0
  webhook:
    url: "https://eval.example.com"
    sample_rate: 0.5
    timeout: 10s
    send_full_content: true
`
	f, err := os.CreateTemp("", "aegisflow-eval-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yamlData)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if !cfg.Eval.Enabled {
		t.Error("expected eval enabled")
	}
	if !cfg.Eval.Builtin.Enabled {
		t.Error("expected builtin eval enabled")
	}
	if cfg.Eval.Builtin.MinResponseTokens != 20 {
		t.Errorf("expected min response tokens 20, got %d", cfg.Eval.Builtin.MinResponseTokens)
	}
	if cfg.Eval.Builtin.LatencyMultiplier != 3.0 {
		t.Errorf("expected latency multiplier 3.0, got %f", cfg.Eval.Builtin.LatencyMultiplier)
	}
	if cfg.Eval.Webhook.URL != "https://eval.example.com" {
		t.Errorf("expected webhook URL, got %s", cfg.Eval.Webhook.URL)
	}
	if cfg.Eval.Webhook.SampleRate != 0.5 {
		t.Errorf("expected sample rate 0.5, got %f", cfg.Eval.Webhook.SampleRate)
	}
	if cfg.Eval.Webhook.Timeout != 10*time.Second {
		t.Errorf("expected webhook timeout 10s, got %v", cfg.Eval.Webhook.Timeout)
	}
	if !cfg.Eval.Webhook.SendFullContent {
		t.Error("expected send full content true")
	}
}

func TestNewWatcher(t *testing.T) {
	// Test with valid file
	f, err := os.CreateTemp("", "aegisflow-watcher-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("server:\n  port: 8080\n")
	f.Close()

	cfg := &Config{}
	w := NewWatcher(f.Name(), cfg, nil)
	if w == nil {
		t.Fatal("expected watcher, got nil")
	}
	if w.path != f.Name() {
		t.Errorf("expected path %s, got %s", f.Name(), w.path)
	}
	got := w.GetConfig()
	if got != cfg {
		t.Error("expected GetConfig to return initial config")
	}
}

func TestNewWatcherMissingFile(t *testing.T) {
	cfg := &Config{}
	w := NewWatcher("/nonexistent/file.yaml", cfg, nil)
	if w == nil {
		t.Fatal("expected watcher even for missing file")
	}
	if !w.lastMod.IsZero() {
		t.Errorf("expected zero lastMod for missing file, got %v", w.lastMod)
	}
}

func TestWatcherStartStop(t *testing.T) {
	f, err := os.CreateTemp("", "aegisflow-watcher-ss-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("server:\n  port: 8080\n")
	f.Close()

	cfg := &Config{Server: ServerConfig{Port: 8080}}
	w := NewWatcher(f.Name(), cfg, nil)
	w.Start(50 * time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	w.Stop()
}

func TestWatcherDetectsChange(t *testing.T) {
	f, err := os.CreateTemp("", "aegisflow-watcher-change-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("server:\n  port: 8080\n")
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	var called atomic.Int32
	w := NewWatcher(f.Name(), cfg, func(c *Config) {
		called.Add(1)
	})

	// Manually modify the file with a new timestamp
	time.Sleep(50 * time.Millisecond)
	if err := os.WriteFile(f.Name(), []byte("server:\n  port: 9999\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Manually call check to trigger reload
	w.check()

	newCfg := w.GetConfig()
	if newCfg.Server.Port != 9999 {
		t.Errorf("expected port 9999 after reload, got %d", newCfg.Server.Port)
	}
	if called.Load() != 1 {
		t.Errorf("expected onChange called once, got %d", called.Load())
	}
}

func TestWatcherCheckNoChange(t *testing.T) {
	f, err := os.CreateTemp("", "aegisflow-watcher-nochange-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("server:\n  port: 8080\n")
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	var called atomic.Int32
	w := NewWatcher(f.Name(), cfg, func(c *Config) {
		called.Add(1)
	})

	// Call check without modifying file - should not trigger onChange
	w.check()
	if called.Load() != 0 {
		t.Errorf("expected onChange not called, got %d", called.Load())
	}
}

func TestWatcherCheckFileDeleted(t *testing.T) {
	f, err := os.CreateTemp("", "aegisflow-watcher-del-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("server:\n  port: 8080\n")
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(f.Name(), cfg, nil)

	// Delete the file, then check should not panic
	os.Remove(f.Name())
	w.check() // should return silently
	if w.GetConfig().Server.Port != 8080 {
		t.Error("config should remain unchanged after file deletion")
	}
}

func TestWatcherCheckInvalidYAMLAfterChange(t *testing.T) {
	f, err := os.CreateTemp("", "aegisflow-watcher-badyaml-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString("server:\n  port: 8080\n")
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	w := NewWatcher(f.Name(), cfg, nil)

	// Write invalid YAML
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(f.Name(), []byte("{{invalid"), 0644)

	w.check()
	// Config should remain unchanged
	if w.GetConfig().Server.Port != 8080 {
		t.Error("config should remain unchanged after invalid YAML reload")
	}
}

func TestFederationConfigParsing(t *testing.T) {
	yamlData := `
federation:
  enabled: true
  mode: "control-plane"
  data_planes:
    - name: "dp1"
      url: "https://dp1.example.com"
      token: "token1"
  control_plane:
    url: "https://cp.example.com"
    token: "cp-token"
    sync_interval: 1m
`
	f, err := os.CreateTemp("", "aegisflow-federation-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(yamlData)
	f.Close()

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if !cfg.Federation.Enabled {
		t.Error("expected federation enabled")
	}
	if cfg.Federation.Mode != "control-plane" {
		t.Errorf("expected mode control-plane, got %s", cfg.Federation.Mode)
	}
	if len(cfg.Federation.DataPlanes) != 1 {
		t.Fatalf("expected 1 data plane, got %d", len(cfg.Federation.DataPlanes))
	}
	if cfg.Federation.DataPlanes[0].Name != "dp1" {
		t.Errorf("expected data plane name dp1, got %s", cfg.Federation.DataPlanes[0].Name)
	}
	if cfg.Federation.ControlPlane.SyncInterval != 1*time.Minute {
		t.Errorf("expected sync interval 1m, got %v", cfg.Federation.ControlPlane.SyncInterval)
	}
}
