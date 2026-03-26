package admin

import (
	_ "embed"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/aegisflow/aegisflow/internal/config"
	"github.com/aegisflow/aegisflow/internal/provider"
	"github.com/aegisflow/aegisflow/internal/usage"
)

//go:embed dashboard.html
var dashboardHTML []byte

type Server struct {
	tracker    *usage.Tracker
	cfg        *config.Config
	registry   *provider.Registry
	requestLog *RequestLog
}

func NewServer(tracker *usage.Tracker, cfg *config.Config, registry *provider.Registry, reqLog *RequestLog) *Server {
	return &Server{tracker: tracker, cfg: cfg, registry: registry, requestLog: reqLog}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)

	r.Get("/health", s.healthHandler)
	r.Get("/metrics", promhttp.Handler().ServeHTTP)
	r.Get("/admin/v1/usage", s.usageHandler)
	r.Get("/admin/v1/providers", s.providersHandler)
	r.Get("/admin/v1/tenants", s.tenantsHandler)
	r.Get("/admin/v1/policies", s.policiesHandler)
	r.Get("/admin/v1/requests", s.requestLog.ServeHTTP)
	r.Get("/dashboard", s.dashboardHandler)
	r.Get("/", s.dashboardHandler)

	return r
}

func (s *Server) GetRequestLog() *RequestLog {
	return s.requestLog
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) usageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.tracker.GetAllUsage())
}

func (s *Server) providersHandler(w http.ResponseWriter, r *http.Request) {
	type providerInfo struct {
		Name    string   `json:"name"`
		Type    string   `json:"type"`
		Enabled bool     `json:"enabled"`
		BaseURL string   `json:"base_url,omitempty"`
		Models  []string `json:"models,omitempty"`
		Healthy bool     `json:"healthy"`
	}

	var providers []providerInfo
	for _, pc := range s.cfg.Providers {
		healthy := false
		if pc.Enabled {
			if p, err := s.registry.Get(pc.Name); err == nil {
				healthy = p.Healthy(r.Context())
			}
		}
		providers = append(providers, providerInfo{
			Name:    pc.Name,
			Type:    pc.Type,
			Enabled: pc.Enabled,
			BaseURL: pc.BaseURL,
			Models:  pc.Models,
			Healthy: healthy,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(providers)
}

func (s *Server) tenantsHandler(w http.ResponseWriter, r *http.Request) {
	type tenantInfo struct {
		ID                string   `json:"id"`
		Name              string   `json:"name"`
		KeyCount          int      `json:"key_count"`
		RequestsPerMinute int      `json:"requests_per_minute"`
		TokensPerMinute   int      `json:"tokens_per_minute"`
		AllowedModels     []string `json:"allowed_models"`
	}

	var tenants []tenantInfo
	for _, t := range s.cfg.Tenants {
		tenants = append(tenants, tenantInfo{
			ID:                t.ID,
			Name:              t.Name,
			KeyCount:          len(t.APIKeys),
			RequestsPerMinute: t.RateLimit.RequestsPerMinute,
			TokensPerMinute:   t.RateLimit.TokensPerMinute,
			AllowedModels:     t.AllowedModels,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tenants)
}

func (s *Server) policiesHandler(w http.ResponseWriter, r *http.Request) {
	type policyInfo struct {
		Name     string   `json:"name"`
		Type     string   `json:"type"`
		Action   string   `json:"action"`
		Phase    string   `json:"phase"`
		Keywords []string `json:"keywords,omitempty"`
		Patterns []string `json:"patterns,omitempty"`
	}

	var policies []policyInfo
	for _, p := range s.cfg.Policies.Input {
		policies = append(policies, policyInfo{
			Name: p.Name, Type: p.Type, Action: p.Action, Phase: "input",
			Keywords: p.Keywords, Patterns: p.Patterns,
		})
	}
	for _, p := range s.cfg.Policies.Output {
		policies = append(policies, policyInfo{
			Name: p.Name, Type: p.Type, Action: p.Action, Phase: "output",
			Keywords: p.Keywords, Patterns: p.Patterns,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(policies)
}

func (s *Server) dashboardHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(dashboardHTML)
}
