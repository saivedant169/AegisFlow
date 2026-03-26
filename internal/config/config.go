package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Providers []ProviderConfig `yaml:"providers"`
	Routes    []RouteConfig   `yaml:"routes"`
	Tenants   []TenantConfig  `yaml:"tenants"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Policies  PoliciesConfig  `yaml:"policies"`
	Telemetry TelemetryConfig `yaml:"telemetry"`
	Logging   LoggingConfig   `yaml:"logging"`
	Cache     CacheConfig     `yaml:"cache"`
	Webhook   WebhookConfig   `yaml:"webhook"`
	Database  DatabaseConfig  `yaml:"database"`
}

type CacheConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Backend  string        `yaml:"backend"`
	TTL      time.Duration `yaml:"ttl"`
	MaxSize  int           `yaml:"max_size"`
	Redis    RedisConfig   `yaml:"redis"`
}

type WebhookConfig struct {
	URL string `yaml:"url"`
}

type DatabaseConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ConnString string `yaml:"conn_string"`
}

type ServerConfig struct {
	Host             string        `yaml:"host"`
	Port             int           `yaml:"port"`
	AdminPort        int           `yaml:"admin_port"`
	ReadTimeout      time.Duration `yaml:"read_timeout"`
	WriteTimeout     time.Duration `yaml:"write_timeout"`
	GracefulShutdown time.Duration `yaml:"graceful_shutdown"`
}

type ProviderConfig struct {
	Name      string            `yaml:"name"`
	Type      string            `yaml:"type"`
	Enabled   bool              `yaml:"enabled"`
	Default   bool              `yaml:"default"`
	BaseURL   string            `yaml:"base_url"`
	APIKeyEnv string            `yaml:"api_key_env"`
	Models    []string          `yaml:"models"`
	Timeout   time.Duration     `yaml:"timeout"`
	MaxRetries int              `yaml:"max_retries"`
	APIVersion string           `yaml:"api_version"`
	Config    map[string]string `yaml:"config"`
}

type RouteConfig struct {
	Match     RouteMatch `yaml:"match"`
	Providers []string   `yaml:"providers"`
	Strategy  string     `yaml:"strategy"`
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
	Name     string   `yaml:"name"`
	Type     string   `yaml:"type"`
	Action   string   `yaml:"action"`
	Keywords []string `yaml:"keywords"`
	Patterns []string `yaml:"patterns"`
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
}

func (c *Config) FindTenantByAPIKey(apiKey string) *TenantConfig {
	for i := range c.Tenants {
		for _, key := range c.Tenants[i].APIKeys {
			if key == apiKey {
				return &c.Tenants[i]
			}
		}
	}
	return nil
}
