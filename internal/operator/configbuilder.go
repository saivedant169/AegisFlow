package operator

import (
	"time"

	"github.com/saivedant169/AegisFlow/internal/config"
)

// ProviderInput represents the data extracted from an AegisFlowProvider CRD.
type ProviderInput struct {
	Name       string
	Type       string
	BaseURL    string
	APIKeyEnv  string
	Models     []string
	Timeout    time.Duration
	MaxRetries int
	Region     string
}

// RouteInput represents the data extracted from an AegisFlowRoute CRD.
type RouteInput struct {
	Model     string
	Providers []string
	Strategy  string
	Regions   []RegionInput
}

// RegionInput represents a geographic routing region within a route.
type RegionInput struct {
	Name      string
	Providers []string
	Strategy  string
}

// TenantInput represents the data extracted from an AegisFlowTenant CRD.
type TenantInput struct {
	ID                string
	Name              string
	APIKeys           []string
	RequestsPerMinute int
	TokensPerMinute   int
	AllowedModels     []string
}

// PolicyInput represents the data extracted from an AegisFlowPolicy CRD.
type PolicyInput struct {
	Name     string
	Phase    string
	Type     string
	Action   string
	Keywords []string
	Patterns []string
	WasmPath string
	Timeout  time.Duration
	OnError  string
}

// GatewayInput represents the data extracted from an AegisFlowGateway CRD.
type GatewayInput struct {
	Port      int
	AdminPort int
	LogLevel  string
	LogFormat string
}

// BuildConfig converts CRD-sourced inputs into a unified config.Config
// that can be marshaled to aegisflow.yaml and stored in a ConfigMap.
func BuildConfig(gw GatewayInput, providers []ProviderInput, routes []RouteInput, tenants []TenantInput, policies []PolicyInput) *config.Config {
	cfg := &config.Config{}

	// Server / gateway settings
	cfg.Server.Host = "0.0.0.0"
	cfg.Server.Port = gw.Port
	cfg.Server.AdminPort = gw.AdminPort
	cfg.Logging.Level = gw.LogLevel
	cfg.Logging.Format = gw.LogFormat

	// Providers
	for _, p := range providers {
		cfg.Providers = append(cfg.Providers, config.ProviderConfig{
			Name:       p.Name,
			Type:       p.Type,
			Enabled:    true,
			BaseURL:    p.BaseURL,
			APIKeyEnv:  p.APIKeyEnv,
			Models:     p.Models,
			Timeout:    p.Timeout,
			MaxRetries: p.MaxRetries,
			Region:     p.Region,
		})
	}

	// Routes
	for _, r := range routes {
		rc := config.RouteConfig{
			Match:     config.RouteMatch{Model: r.Model},
			Providers: r.Providers,
			Strategy:  r.Strategy,
		}
		for _, reg := range r.Regions {
			rc.Regions = append(rc.Regions, config.RegionConfig{
				Name:      reg.Name,
				Providers: reg.Providers,
				Strategy:  reg.Strategy,
			})
		}
		cfg.Routes = append(cfg.Routes, rc)
	}

	// Tenants
	for _, t := range tenants {
		apiKeys := make([]config.APIKeyEntry, len(t.APIKeys))
		for j, k := range t.APIKeys {
			apiKeys[j] = config.APIKeyEntry{Key: k, Role: "operator"}
		}
		cfg.Tenants = append(cfg.Tenants, config.TenantConfig{
			ID:      t.ID,
			Name:    t.Name,
			APIKeys: apiKeys,
			RateLimit: config.TenantRateLimit{
				RequestsPerMinute: t.RequestsPerMinute,
				TokensPerMinute:   t.TokensPerMinute,
			},
			AllowedModels: t.AllowedModels,
		})
	}

	// Policies
	for _, p := range policies {
		pc := config.PolicyConfig{
			Name:     p.Name,
			Type:     p.Type,
			Action:   p.Action,
			Keywords: p.Keywords,
			Patterns: p.Patterns,
			Path:     p.WasmPath,
			Timeout:  p.Timeout,
			OnError:  p.OnError,
		}
		if p.Phase == "input" {
			cfg.Policies.Input = append(cfg.Policies.Input, pc)
		} else {
			cfg.Policies.Output = append(cfg.Policies.Output, pc)
		}
	}

	return cfg
}
