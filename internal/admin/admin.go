package admin

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/aegisflow/aegisflow/internal/cache"
	"github.com/aegisflow/aegisflow/internal/config"
	"github.com/aegisflow/aegisflow/internal/middleware"
	"github.com/aegisflow/aegisflow/internal/provider"
	"github.com/aegisflow/aegisflow/internal/usage"
)

// AnalyticsProvider is the interface consumed by the admin API to avoid an
// import cycle with the analytics package. Use analytics.NewAdminAdapter to
// wrap a *Collector + *AlertManager so it satisfies this interface.
type AnalyticsProvider interface {
	RealtimeSummary() map[string]interface{}
	RecentAlerts(limit int) interface{}
	AcknowledgeAlert(id string) bool
	Dimensions() []string
}

// BudgetProvider is the interface consumed by the admin API to avoid an import
// cycle with the budget package. Use budget.NewAdminAdapter to wrap a
// *budget.Manager so it satisfies this interface.
type BudgetProvider interface {
	AllStatuses() interface{}
	ForecastAll() interface{}
}

// RolloutManager is the interface consumed by the admin API to avoid an import
// cycle with the rollout package. Use rollout.NewAdminAdapter to wrap a
// *rollout.Manager so it satisfies this interface.
type RolloutManager interface {
	ListRollouts() (any, error)
	CreateRollout(routeModel string, baselineProviders []string, canaryProvider string, stages []int, observationWindow time.Duration, errorThreshold float64, latencyP95Threshold int64) (any, error)
	GetRolloutWithMetrics(id string) (any, error)
	PauseRollout(id string) error
	ResumeRollout(id string) error
	RollbackRollout(id string) error
}

//go:embed dashboard.html
var dashboardHTML []byte

type Server struct {
	tracker    *usage.Tracker
	cfg        *config.Config
	registry   *provider.Registry
	requestLog *RequestLog
	cache              cache.Cache
	rolloutMgr         RolloutManager
	analyticsProvider  AnalyticsProvider
	budgetProvider     BudgetProvider
}

func NewServer(tracker *usage.Tracker, cfg *config.Config, registry *provider.Registry, reqLog *RequestLog, c cache.Cache, rm RolloutManager, ap AnalyticsProvider, bp BudgetProvider) *Server {
	return &Server{tracker: tracker, cfg: cfg, registry: registry, requestLog: reqLog, cache: c, rolloutMgr: rm, analyticsProvider: ap, budgetProvider: bp}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(middleware.CORS(s.cfg))

	r.Get("/health", s.healthHandler)
	r.Get("/metrics", promhttp.Handler().ServeHTTP)
	r.Get("/admin/v1/usage", s.usageHandler)
	r.Get("/admin/v1/providers", s.providersHandler)
	r.Get("/admin/v1/tenants", s.tenantsHandler)
	r.Get("/admin/v1/policies", s.policiesHandler)
	r.Get("/admin/v1/requests", s.requestLog.ServeHTTP)
	r.Get("/admin/v1/violations", s.violationsHandler)
	r.Get("/admin/v1/cache", s.cacheHandler)
	r.Get("/dashboard", s.dashboardHandler)
	r.Get("/", s.dashboardHandler)

	// Analytics and alerts endpoints
	r.Get("/admin/v1/analytics", s.analyticsHandler)
	r.Get("/admin/v1/analytics/realtime", s.analyticsRealtimeHandler)
	r.Get("/admin/v1/alerts", s.alertsHandler)
	r.Post("/admin/v1/alerts/{id}/acknowledge", s.alertAcknowledgeHandler)

	// Budget endpoints
	r.Get("/admin/v1/budgets", s.budgetsHandler)

	// Rollout management endpoints
	r.Get("/admin/v1/rollouts", s.rolloutsListHandler)
	r.Post("/admin/v1/rollouts", s.rolloutsCreateHandler)
	r.Get("/admin/v1/rollouts/{id}", s.rolloutGetHandler)
	r.Post("/admin/v1/rollouts/{id}/pause", s.rolloutPauseHandler)
	r.Post("/admin/v1/rollouts/{id}/resume", s.rolloutResumeHandler)
	r.Post("/admin/v1/rollouts/{id}/rollback", s.rolloutRollbackHandler)

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
		Region  string   `json:"region,omitempty"`
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
			Region:  pc.Region,
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

func (s *Server) violationsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.requestLog.RecentViolations(100))
}

func (s *Server) cacheHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.cache != nil {
		json.NewEncoder(w).Encode(s.cache.Stats())
	} else {
		json.NewEncoder(w).Encode(cache.CacheStats{})
	}
}

func (s *Server) dashboardHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(dashboardHTML)
}

// --- Rollout handlers ---

func (s *Server) rolloutUnavailable(w http.ResponseWriter) bool {
	if s.rolloutMgr == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "rollout manager not available"})
		return true
	}
	return false
}

func (s *Server) rolloutsListHandler(w http.ResponseWriter, r *http.Request) {
	if s.rolloutUnavailable(w) {
		return
	}
	result, err := s.rolloutMgr.ListRollouts()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	if result == nil {
		result = []any{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type createRolloutRequest struct {
	RouteModel          string  `json:"route_model"`
	CanaryProvider      string  `json:"canary_provider"`
	Stages              []int   `json:"stages"`
	ObservationWindow   string  `json:"observation_window"`
	ErrorThreshold      float64 `json:"error_threshold"`
	LatencyP95Threshold int64   `json:"latency_p95_threshold"`
}

func (s *Server) rolloutsCreateHandler(w http.ResponseWriter, r *http.Request) {
	if s.rolloutUnavailable(w) {
		return
	}
	var req createRolloutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}

	obsWindow, err := time.ParseDuration(req.ObservationWindow)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid observation_window: " + err.Error()})
		return
	}

	// Find baseline providers from config routes matching the model.
	var baselineProviders []string
	for _, route := range s.cfg.Routes {
		if strings.EqualFold(route.Match.Model, req.RouteModel) {
			baselineProviders = route.Providers
			break
		}
	}

	created, err := s.rolloutMgr.CreateRollout(
		req.RouteModel,
		baselineProviders,
		req.CanaryProvider,
		req.Stages,
		obsWindow,
		req.ErrorThreshold,
		req.LatencyP95Threshold,
	)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (s *Server) rolloutGetHandler(w http.ResponseWriter, r *http.Request) {
	if s.rolloutUnavailable(w) {
		return
	}
	id := chi.URLParam(r, "id")
	result, err := s.rolloutMgr.GetRolloutWithMetrics(id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) rolloutPauseHandler(w http.ResponseWriter, r *http.Request) {
	if s.rolloutUnavailable(w) {
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.rolloutMgr.PauseRollout(id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) rolloutResumeHandler(w http.ResponseWriter, r *http.Request) {
	if s.rolloutUnavailable(w) {
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.rolloutMgr.ResumeRollout(id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) rolloutRollbackHandler(w http.ResponseWriter, r *http.Request) {
	if s.rolloutUnavailable(w) {
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.rolloutMgr.RollbackRollout(id); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// --- Budget handlers ---

func (s *Server) budgetsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.budgetProvider == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"statuses":  []interface{}{},
			"forecasts": []interface{}{},
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"statuses":  s.budgetProvider.AllStatuses(),
		"forecasts": s.budgetProvider.ForecastAll(),
	})
}

// --- Analytics handlers ---

func (s *Server) analyticsUnavailable(w http.ResponseWriter) bool {
	if s.analyticsProvider == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "analytics not enabled"})
		return true
	}
	return false
}

func (s *Server) analyticsHandler(w http.ResponseWriter, r *http.Request) {
	if s.analyticsUnavailable(w) {
		return
	}
	dims := s.analyticsProvider.Dimensions()
	summary := s.analyticsProvider.RealtimeSummary()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"dimensions": dims,
		"summary":    summary,
	})
}

func (s *Server) analyticsRealtimeHandler(w http.ResponseWriter, r *http.Request) {
	if s.analyticsUnavailable(w) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.analyticsProvider.RealtimeSummary())
}

func (s *Server) alertsHandler(w http.ResponseWriter, r *http.Request) {
	if s.analyticsUnavailable(w) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.analyticsProvider.RecentAlerts(100))
}

func (s *Server) alertAcknowledgeHandler(w http.ResponseWriter, r *http.Request) {
	if s.analyticsUnavailable(w) {
		return
	}
	id := chi.URLParam(r, "id")
	if s.analyticsProvider.AcknowledgeAlert(id) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "alert not found"})
	}
}
