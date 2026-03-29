package config

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig     `yaml:"server"`
	Providers []ProviderConfig `yaml:"providers"`
	Routes    []RouteConfig    `yaml:"routes"`
	Tenants   []TenantConfig   `yaml:"tenants"`
	RateLimit RateLimitConfig  `yaml:"rate_limit"`
	Policies  PoliciesConfig   `yaml:"policies"`
	Telemetry TelemetryConfig  `yaml:"telemetry"`
	Logging   LoggingConfig    `yaml:"logging"`
	Cache     CacheConfig      `yaml:"cache"`
	Webhook   WebhookConfig    `yaml:"webhook"`
	Database  DatabaseConfig   `yaml:"database"`
	Admin     AdminConfig      `yaml:"admin"`
	Aliases   AliasConfig      `yaml:"aliases"`
	Transform TransformConfig  `yaml:"transform"`
	Analytics AnalyticsConfig  `yaml:"analytics"`
	Budgets   BudgetsConfig    `yaml:"budgets"`
}

type CacheConfig struct {
	Enabled bool          `yaml:"enabled"`
	Backend string        `yaml:"backend"`
	TTL     time.Duration `yaml:"ttl"`
	MaxSize int           `yaml:"max_size"`
	Redis   RedisConfig   `yaml:"redis"`
}

type WebhookConfig struct {
	URL string `yaml:"url"`
}

type DatabaseConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ConnString string `yaml:"conn_string"`
}

type AdminConfig struct {
	Token string `yaml:"token"`
}

type AliasConfig struct {
	Models map[string]string `yaml:"models"`
}

type TransformConfig struct {
	SystemPromptPrefix  string `yaml:"system_prompt_prefix"`
	SystemPromptSuffix  string `yaml:"system_prompt_suffix"`
	DefaultSystemPrompt string `yaml:"default_system_prompt"`
}

type AnalyticsConfig struct {
	Enabled          bool          `yaml:"enabled"`
	RetentionHours   int           `yaml:"retention_hours"`
	FlushInterval    time.Duration `yaml:"flush_interval"`
	AnomalyDetection AnomalyConfig `yaml:"anomaly_detection"`
}

type AnomalyConfig struct {
	Enabled            bool             `yaml:"enabled"`
	EvaluationInterval time.Duration    `yaml:"evaluation_interval"`
	Static             StaticThresholds `yaml:"static"`
	Baseline           BaselineConfig   `yaml:"baseline"`
}

type StaticThresholds struct {
	ErrorRateMax         float64 `yaml:"error_rate_max"`
	P95LatencyMax        int64   `yaml:"p95_latency_max"`
	RequestsPerMinuteMax int64   `yaml:"requests_per_minute_max"`
	CostPerMinuteMax     float64 `yaml:"cost_per_minute_max"`
}

type BaselineConfig struct {
	Window          time.Duration `yaml:"window"`
	StddevThreshold float64       `yaml:"stddev_threshold"`
}

type BudgetsConfig struct {
	Enabled bool                    `yaml:"enabled"`
	Global  BudgetLimitConfig       `yaml:"global"`
	Tenants map[string]TenantBudget `yaml:"tenants"`
}

type BudgetLimitConfig struct {
	Monthly float64 `yaml:"monthly"`
	Daily   float64 `yaml:"daily"`
	AlertAt int     `yaml:"alert_at"`
	WarnAt  int     `yaml:"warn_at"`
}

type TenantBudget struct {
	Monthly float64                      `yaml:"monthly"`
	Daily   float64                      `yaml:"daily"`
	AlertAt int                          `yaml:"alert_at"`
	WarnAt  int                          `yaml:"warn_at"`
	Models  map[string]BudgetLimitConfig `yaml:"models"`
}

type ServerConfig struct {
	Host             string        `yaml:"host"`
	Port             int           `yaml:"port"`
	AdminPort        int           `yaml:"admin_port"`
	ReadTimeout      time.Duration `yaml:"read_timeout"`
	WriteTimeout     time.Duration `yaml:"write_timeout"`
	GracefulShutdown time.Duration `yaml:"graceful_shutdown"`
	MaxBodySize      int64         `yaml:"max_body_size"`
	CORS             CORSConfig    `yaml:"cors"`
}

type CORSConfig struct {
	Enabled          bool     `yaml:"enabled"`
	AllowedOrigins   []string `yaml:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers"`
	ExposedHeaders   []string `yaml:"exposed_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
	MaxAge           int      `yaml:"max_age"`
}

type ProviderConfig struct {
	Name       string            `yaml:"name"`
	Type       string            `yaml:"type"`
	Enabled    bool              `yaml:"enabled"`
	Default    bool              `yaml:"default"`
	BaseURL    string            `yaml:"base_url"`
	APIKeyEnv  string            `yaml:"api_key_env"`
	Models     []string          `yaml:"models"`
	Timeout    time.Duration     `yaml:"timeout"`
	MaxRetries int               `yaml:"max_retries"`
	APIVersion string            `yaml:"api_version"`
	Config     map[string]string `yaml:"config"`
	Region     string            `yaml:"region"`
}

type RegionConfig struct {
	Name      string   `yaml:"name"`
	Providers []string `yaml:"providers"`
	Strategy  string   `yaml:"strategy"`
}

type RouteConfig struct {
	Match     RouteMatch     `yaml:"match"`
	Providers []string       `yaml:"providers"`
	Strategy  string         `yaml:"strategy"`
	Canary    *CanaryConfig  `yaml:"canary,omitempty"`
	Regions   []RegionConfig `yaml:"regions,omitempty"`
}

type CanaryConfig struct {
	TargetProvider      string        `yaml:"target_provider"`
	Stages              []int         `yaml:"stages"`
	ObservationWindow   time.Duration `yaml:"observation_window"`
	ErrorThreshold      float64       `yaml:"error_threshold"`
	LatencyP95Threshold int64         `yaml:"latency_p95_threshold"`
}

type RouteMatch struct {
	Model string `yaml:"model"`
}

type TenantConfig struct {
	ID            string          `yaml:"id"`
	Name          string          `yaml:"name"`
	APIKeys       []string        `yaml:"api_keys"`
	RateLimit     TenantRateLimit `yaml:"rate_limit"`
	AllowedModels []string        `yaml:"allowed_models"`
}

type TenantRateLimit struct {
	RequestsPerMinute int `yaml:"requests_per_minute"`
	TokensPerMinute   int `yaml:"tokens_per_minute"`
}

type RateLimitConfig struct {
	Backend string      `yaml:"backend"`
	Redis   RedisConfig `yaml:"redis"`
}

type RedisConfig struct {
	Address  string `yaml:"address"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type PoliciesConfig struct {
	Input  []PolicyConfig `yaml:"input"`
	Output []PolicyConfig `yaml:"output"`
}

type PolicyConfig struct {
	Name     string        `yaml:"name"`
	Type     string        `yaml:"type"`
	Action   string        `yaml:"action"`
	Keywords []string      `yaml:"keywords"`
	Patterns []string      `yaml:"patterns"`
	Path     string        `yaml:"path"`
	Timeout  time.Duration `yaml:"timeout"`
	OnError  string        `yaml:"on_error"`
}

type TelemetryConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Exporter string        `yaml:"exporter"`
	OTLP     OTLPConfig    `yaml:"otlp"`
	Metrics  MetricsConfig `yaml:"metrics"`
}

type OTLPConfig struct {
	Endpoint string `yaml:"endpoint"`
	Insecure bool   `yaml:"insecure"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	setDefaults(cfg)
	return cfg, nil
}

func setDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.AdminPort == 0 {
		cfg.Server.AdminPort = 8081
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 120 * time.Second
	}
	if cfg.Server.GracefulShutdown == 0 {
		cfg.Server.GracefulShutdown = 10 * time.Second
	}
	if cfg.RateLimit.Backend == "" {
		cfg.RateLimit.Backend = "memory"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	if cfg.Telemetry.Exporter == "" {
		cfg.Telemetry.Exporter = "stdout"
	}
	if cfg.Telemetry.Metrics.Path == "" {
		cfg.Telemetry.Metrics.Path = "/metrics"
	}
	if cfg.Cache.Backend == "" {
		cfg.Cache.Backend = "memory"
	}
	if cfg.Cache.TTL == 0 {
		cfg.Cache.TTL = 5 * time.Minute
	}
	if cfg.Cache.MaxSize == 0 {
		cfg.Cache.MaxSize = 1000
	}
	if cfg.Analytics.RetentionHours == 0 {
		cfg.Analytics.RetentionHours = 48
	}
	if cfg.Analytics.FlushInterval == 0 {
		cfg.Analytics.FlushInterval = time.Hour
	}
	if cfg.Analytics.AnomalyDetection.EvaluationInterval == 0 {
		cfg.Analytics.AnomalyDetection.EvaluationInterval = time.Minute
	}
	if cfg.Analytics.AnomalyDetection.Static.ErrorRateMax == 0 {
		cfg.Analytics.AnomalyDetection.Static.ErrorRateMax = 20
	}
	if cfg.Analytics.AnomalyDetection.Static.P95LatencyMax == 0 {
		cfg.Analytics.AnomalyDetection.Static.P95LatencyMax = 5000
	}
	if cfg.Analytics.AnomalyDetection.Static.RequestsPerMinuteMax == 0 {
		cfg.Analytics.AnomalyDetection.Static.RequestsPerMinuteMax = 10000
	}
	if cfg.Analytics.AnomalyDetection.Static.CostPerMinuteMax == 0 {
		cfg.Analytics.AnomalyDetection.Static.CostPerMinuteMax = 50.0
	}
	if cfg.Analytics.AnomalyDetection.Baseline.Window == 0 {
		cfg.Analytics.AnomalyDetection.Baseline.Window = 24 * time.Hour
	}
	if cfg.Analytics.AnomalyDetection.Baseline.StddevThreshold == 0 {
		cfg.Analytics.AnomalyDetection.Baseline.StddevThreshold = 3
	}
	if cfg.Budgets.Global.AlertAt == 0 {
		cfg.Budgets.Global.AlertAt = 80
	}
	if cfg.Budgets.Global.WarnAt == 0 {
		cfg.Budgets.Global.WarnAt = 90
	}

	// CORS defaults
	if cfg.Server.CORS.Enabled {
		if len(cfg.Server.CORS.AllowedOrigins) == 0 {
			cfg.Server.CORS.AllowedOrigins = []string{"*"}
		}
		if len(cfg.Server.CORS.AllowedMethods) == 0 {
			cfg.Server.CORS.AllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
		}
		if len(cfg.Server.CORS.AllowedHeaders) == 0 {
			cfg.Server.CORS.AllowedHeaders = []string{"Authorization", "Content-Type", "X-API-Key"}
		}
		if cfg.Server.CORS.MaxAge == 0 {
			cfg.Server.CORS.MaxAge = 86400
		}
	}
}

func (c *Config) FindTenantByAPIKey(apiKey string) *TenantConfig {
	// Use constant-time comparison to prevent timing attacks.
	// Hash both sides so length differences don't leak info.
	inputHash := sha256.Sum256([]byte(apiKey))
	var match *TenantConfig
	for i := range c.Tenants {
		for _, key := range c.Tenants[i].APIKeys {
			keyHash := sha256.Sum256([]byte(key))
			if subtle.ConstantTimeCompare(inputHash[:], keyHash[:]) == 1 {
				match = &c.Tenants[i]
			}
		}
	}
	return match
}
