package admin

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/saivedant169/AegisFlow/internal/cache"
	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/usage"
	"github.com/saivedant169/AegisFlow/pkg/types"
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

// AuditProvider is the interface consumed by the admin API to avoid an import
// cycle with the audit package. Use audit.NewAdminAdapter to wrap a
// *audit.Logger so it satisfies this interface.
type AuditProvider interface {
	Query(actor, actorRole, action, tenantID string, limit int) (interface{}, error)
	Verify() (interface{}, error)
	Log(actor, actorRole, action, resource, detail, tenantID, model string)
}

// FederationProvider is the interface consumed by the admin API to avoid an
// import cycle with the federation package.
type FederationProvider interface {
	ConfigHandler(w http.ResponseWriter, r *http.Request)
	MetricsHandler(w http.ResponseWriter, r *http.Request)
	StatusHandler(w http.ResponseWriter, r *http.Request)
	PlanesHandler(w http.ResponseWriter, r *http.Request)
}

// CostOptProvider is the interface consumed by the admin API to avoid an
// import cycle with the costopt package. Use costopt.NewAdminAdapter to
// wrap a *costopt.Engine so it satisfies this interface.
type CostOptProvider interface {
	Recommendations() interface{}
}

// ApprovalProvider is the interface consumed by the admin API to avoid an
// import cycle with the approval package. Use approval.NewAdminAdapter to
// wrap a *approval.Queue so it satisfies this interface.
type ApprovalProvider interface {
	Pending() interface{}
	History(limit int) interface{}
	Get(id string) (interface{}, error)
	Approve(id, reviewer, comment string) (interface{}, error)
	Deny(id, reviewer, comment string) (interface{}, error)
	Submit(env interface{}) (string, error)
}

// CredentialProvider is the interface consumed by the admin API to avoid an
// import cycle with the credential package. Use credential.NewAdminAdapter to
// wrap a *credential.Registry so it satisfies this interface.
type CredentialProvider interface {
	ActiveCredentials() interface{}
	RevokeCredential(id string) error
	// IssueCredential issues a credential and returns its provenance metadata
	// (never the secret). The provenance is suitable for embedding in evidence
	// records and API responses.
	IssueCredential(providerName, taskID, target, capability, envelopeID string) (interface{}, error)
}

// EvidenceProvider is the interface consumed by the admin API to avoid an
// import cycle with the evidence package. Implementations provide session
// evidence chain export and verification.
type EvidenceProvider interface {
	ExportSession(sessionID string) (interface{}, error)
	VerifySession(sessionID string) (interface{}, error)
	ListSessions() interface{}
	RenderReport(sessionID string) (string, error)
	RenderHTMLReport(sessionID string) (string, error)
}

// CapabilityProvider is the interface consumed by the admin API to avoid an
// import cycle with the capability package. Use capability.NewAdminAdapter to
// wrap a *capability.Issuer so it satisfies this interface.
type CapabilityProvider interface {
	ActiveTickets() interface{}
	RevokeTicket(id string) error
	VerifyTicket(id string) (interface{}, error)
}

// ToolPolicyProvider is the interface consumed by the admin API to avoid an
// import cycle with the toolpolicy package. Implementations evaluate
// ActionEnvelopes against tool policy rules.
type ToolPolicyProvider interface {
	Evaluate(env *envelope.ActionEnvelope) string // returns "allow", "review", or "block"
	// EvaluateWithTrace returns a JSON-serializable decision trace. Returns nil
	// if the provider does not support tracing.
	EvaluateWithTrace(env *envelope.ActionEnvelope) interface{}
}

// ManifestProvider is the interface consumed by the admin API to avoid an
// import cycle with the manifest package. Use manifest.NewAdminAdapter to
// wrap a *manifest.Store + *manifest.DriftDetector so it satisfies this interface.
type ManifestProvider interface {
	Register(m interface{}) error
	Get(id string) (interface{}, error)
	List() interface{}
	Deactivate(id string) error
	GetDrift(id string) interface{}
	CheckDrift(taskID string, env *envelope.ActionEnvelope, actionCount int, currentBudget float64) interface{}
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

// SupplyChainProvider is the interface consumed by the admin API to list
// loaded supply chain assets and their trust status.
type SupplyChainProvider interface {
	ListAssets() interface{}
}

// BehavioralProvider is the interface consumed by the admin API to avoid an
// import cycle with the behavioral package. Use behavioral.NewAdminAdapter to
// wrap a *behavioral.Registry so it satisfies this interface.
type BehavioralProvider interface {
	SessionRisk(sessionID string) (interface{}, error)
	ListSessions() interface{}
}

// ResilienceProvider is the interface consumed by the admin API to expose
// health monitoring, degradation modes, retention stats, and backup endpoints.
// Use resilience.NewAdminAdapter to satisfy this interface.
type ResilienceProvider interface {
	DetailedHealth() interface{}
	DegradationModes() interface{}
	CreateBackup() (interface{}, error)
	ListBackups() interface{}
	RetentionStats() interface{}
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
	auditProvider      AuditProvider
	federationProvider FederationProvider
	costOptProvider    CostOptProvider
	evidenceProvider   EvidenceProvider
	approvalProvider   ApprovalProvider
	credentialProvider CredentialProvider
	toolPolicyProvider   ToolPolicyProvider
	manifestProvider     ManifestProvider
	capabilityProvider   CapabilityProvider
	supplyChainProvider  SupplyChainProvider
	behavioralProvider   BehavioralProvider
	resilienceProvider   ResilienceProvider
}

func NewServer(tracker *usage.Tracker, cfg *config.Config, registry *provider.Registry, reqLog *RequestLog, c cache.Cache, rm RolloutManager, ap AnalyticsProvider, bp BudgetProvider, aup AuditProvider, fp FederationProvider, cop CostOptProvider, ep EvidenceProvider, apvp ApprovalProvider, crp CredentialProvider, opts ...ServerOption) *Server {
	s := &Server{tracker: tracker, cfg: cfg, registry: registry, requestLog: reqLog, cache: c, rolloutMgr: rm, analyticsProvider: ap, budgetProvider: bp, auditProvider: aup, federationProvider: fp, costOptProvider: cop, evidenceProvider: ep, approvalProvider: apvp, credentialProvider: crp}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ServerOption is a functional option for configuring Server.
type ServerOption func(*Server)

// WithToolPolicyProvider sets the tool policy provider on the admin server.
func WithToolPolicyProvider(tp ToolPolicyProvider) ServerOption {
	return func(s *Server) {
		s.toolPolicyProvider = tp
	}
}

// WithManifestProvider sets the manifest provider on the admin server.
func WithManifestProvider(mp ManifestProvider) ServerOption {
	return func(s *Server) {
		s.manifestProvider = mp
	}
}

// WithCapabilityProvider sets the capability ticket provider on the admin server.
func WithCapabilityProvider(cp CapabilityProvider) ServerOption {
	return func(s *Server) {
		s.capabilityProvider = cp
	}
}

// WithSupplyChainProvider sets the supply chain asset provider on the admin server.
func WithSupplyChainProvider(scp SupplyChainProvider) ServerOption {
	return func(s *Server) {
		s.supplyChainProvider = scp
	}
}

// WithBehavioralProvider sets the behavioral analysis provider on the admin server.
func WithBehavioralProvider(bp BehavioralProvider) ServerOption {
	return func(s *Server) {
		s.behavioralProvider = bp
	}
}

// WithResilienceProvider sets the resilience (health/degradation/backup/retention) provider.
func WithResilienceProvider(rp ResilienceProvider) ServerOption {
	return func(s *Server) {
		s.resilienceProvider = rp
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	r.Use(middleware.CORS(s.cfg))
	r.Use(middleware.SoftAuth(s.cfg))

	// Public — no auth required
	r.Get("/health", s.healthHandler)
	r.Get("/metrics", promhttp.Handler().ServeHTTP)
	r.Get("/dashboard", s.dashboardHandler)
	r.Get("/", s.dashboardHandler)

	// Read-only endpoints — accessible without API key on admin port.
	// The admin port (8081) should not be publicly exposed. These endpoints
	// are open so the embedded dashboard can fetch data without auth.
	r.Get("/admin/v1/usage", s.usageHandler)
	r.Get("/admin/v1/providers", s.providersHandler)
	r.Get("/admin/v1/tenants", s.tenantsHandler)
	r.Get("/admin/v1/policies", s.policiesHandler)
	r.Get("/admin/v1/requests", s.requestLog.ServeHTTP)
	r.Get("/admin/v1/violations", s.violationsHandler)
	r.Get("/admin/v1/cache", s.cacheHandler)
	r.Get("/admin/v1/analytics", s.analyticsHandler)
	r.Get("/admin/v1/analytics/realtime", s.analyticsRealtimeHandler)
	r.Get("/admin/v1/alerts", s.alertsHandler)
	r.Get("/admin/v1/budgets", s.budgetsHandler)
	r.Get("/admin/v1/cost-recommendations", s.handleCostRecommendations)
	r.Get("/admin/v1/rollouts", s.rolloutsListHandler)
	r.Get("/admin/v1/rollouts/{id}", s.rolloutGetHandler)
	r.Get("/admin/v1/whoami", s.whoamiHandler)
	r.Get("/admin/v1/approvals", s.handleApprovalsPending)
	r.Get("/admin/v1/approvals/history", s.handleApprovalsHistory)
	r.Get("/admin/v1/approvals/{id}", s.handleApprovalGet)
	r.Get("/admin/v1/credentials", s.handleCredentialsList)
	r.Get("/admin/v1/tickets", s.handleTicketsList)
	r.Get("/admin/v1/tickets/{id}/verify", s.handleTicketVerify)
	r.Get("/admin/v1/evidence/sessions", s.handleEvidenceSessions)
	r.Get("/admin/v1/evidence/sessions/{id}/export", s.handleEvidenceExport)
	r.Post("/admin/v1/evidence/sessions/{id}/verify", s.handleEvidenceVerify)
	r.Get("/admin/v1/evidence/sessions/{id}/report", s.handleEvidenceReport)
	r.Get("/admin/v1/evidence/sessions/{id}/report.html", s.handleEvidenceReportHTML)
	r.Post("/admin/v1/test-action", s.handleTestAction)
	r.Post("/admin/v1/simulate", s.handleSimulate)
	r.Get("/admin/v1/actions/{id}/why", s.handleActionWhy)
	r.Get("/admin/v1/manifests", s.handleManifestList)
	r.Get("/admin/v1/manifests/{id}", s.handleManifestGet)
	r.Get("/admin/v1/manifests/{id}/drift", s.handleManifestDrift)
	r.Post("/admin/v1/manifests", s.handleManifestCreate)
	r.Delete("/admin/v1/manifests/{id}", s.handleManifestDeactivate)
	r.Get("/admin/v1/supply-chain", s.handleSupplyChain)
	r.Get("/admin/v1/sessions/{id}/risk", s.handleSessionRisk)
	r.Get("/admin/v1/health/detailed", s.handleHealthDetailed)
	r.Get("/admin/v1/resilience/degradation", s.handleResilienceDegradation)
	r.Get("/admin/v1/resilience/backups", s.handleResilienceBackupsList)
	r.Get("/admin/v1/resilience/retention", s.handleResilienceRetention)
	r.Post("/admin/v1/resilience/backup", s.handleResilienceBackupCreate)
	// GraphQL endpoint (reuses the same provider interfaces as REST)
	if s.cfg.Admin.GraphQL.Enabled {
		schema, err := s.buildSchema()
		if err == nil {
			r.Post("/admin/v1/graphql", s.graphqlHandler(schema))
		}
	}

	if s.federationProvider != nil {
		r.Get("/admin/v1/federation/config", s.federationProvider.ConfigHandler)
		r.Post("/admin/v1/federation/metrics", s.federationProvider.MetricsHandler)
		r.Post("/admin/v1/federation/status", s.federationProvider.StatusHandler)
		r.Get("/admin/v1/federation/planes", s.federationProvider.PlanesHandler)
	}

	// Operator — state changes (requires operator or admin role)
	r.Group(func(r chi.Router) {
		r.Use(middleware.RBAC("operator"))
		r.Post("/admin/v1/rollouts", s.rolloutsCreateHandler)
		r.Post("/admin/v1/rollouts/{id}/pause", s.rolloutPauseHandler)
		r.Post("/admin/v1/rollouts/{id}/resume", s.rolloutResumeHandler)
		r.Post("/admin/v1/rollouts/{id}/rollback", s.rolloutRollbackHandler)
		r.Post("/admin/v1/alerts/{id}/acknowledge", s.alertAcknowledgeHandler)
		r.Post("/admin/v1/approvals/{id}/approve", s.handleApprovalApprove)
		r.Post("/admin/v1/approvals/{id}/deny", s.handleApprovalDeny)
		r.Post("/admin/v1/credentials/{id}/revoke", s.handleCredentialRevoke)
		r.Post("/admin/v1/tickets/{id}/revoke", s.handleTicketRevoke)
		r.Get("/admin/v1/audit", s.auditHandler)
	})

	// Admin — audit integrity verification
	r.Group(func(r chi.Router) {
		r.Use(middleware.RBAC("admin"))
		r.Post("/admin/v1/audit/verify", s.auditVerifyHandler)
	})

	return r
}

func (s *Server) GetRequestLog() *RequestLog {
	return s.requestLog
}

// writeAPIError writes a structured error response using the standard
// types.ErrorResponse format, consistent with the gateway and middleware.
func writeAPIError(w http.ResponseWriter, code int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(types.NewErrorResponse(code, errType, message))
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
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "rollout manager not available")
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
		writeAPIError(w, http.StatusInternalServerError, "server_error", err.Error())
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
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid JSON: " + err.Error())
		return
	}

	obsWindow, err := time.ParseDuration(req.ObservationWindow)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid observation_window: " + err.Error())
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
		writeAPIError(w, http.StatusConflict, "conflict", err.Error())
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
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
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
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
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
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
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
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
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

// --- Cost optimization handlers ---

func (s *Server) handleCostRecommendations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.costOptProvider == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"recommendations": []interface{}{}})
		return
	}
	recs := s.costOptProvider.Recommendations()
	json.NewEncoder(w).Encode(map[string]interface{}{"recommendations": recs})
}

// --- Analytics handlers ---

func (s *Server) analyticsUnavailable(w http.ResponseWriter) bool {
	if s.analyticsProvider == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "analytics not enabled")
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
		writeAPIError(w, http.StatusNotFound, "not_found", "alert not found")
	}
}

// --- Audit handlers ---

func (s *Server) auditHandler(w http.ResponseWriter, r *http.Request) {
	if s.auditProvider == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}
	actor := r.URL.Query().Get("actor")
	actorRole := r.URL.Query().Get("actor_role")
	action := r.URL.Query().Get("action")
	tenantID := r.URL.Query().Get("tenant_id")
	limit := 100 // default
	result, err := s.auditProvider.Query(actor, actorRole, action, tenantID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) auditVerifyHandler(w http.ResponseWriter, r *http.Request) {
	if s.auditProvider == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"valid": true, "message": "audit not configured"})
		return
	}
	result, err := s.auditProvider.Verify()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// --- Whoami handler ---

func (s *Server) whoamiHandler(w http.ResponseWriter, r *http.Request) {
	role := middleware.RoleFromContext(r.Context())
	tenant := middleware.TenantFromContext(r.Context())
	resp := map[string]string{"role": role}
	if tenant != nil {
		resp["tenant_id"] = tenant.ID
		resp["tenant_name"] = tenant.Name
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// --- Evidence handlers ---

func (s *Server) evidenceUnavailable(w http.ResponseWriter) bool {
	if s.evidenceProvider == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "evidence provider not available")
		return true
	}
	return false
}

func (s *Server) handleEvidenceSessions(w http.ResponseWriter, r *http.Request) {
	if s.evidenceUnavailable(w) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.evidenceProvider.ListSessions())
}

func (s *Server) handleEvidenceExport(w http.ResponseWriter, r *http.Request) {
	if s.evidenceUnavailable(w) {
		return
	}
	id := chi.URLParam(r, "id")
	result, err := s.evidenceProvider.ExportSession(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleEvidenceVerify(w http.ResponseWriter, r *http.Request) {
	if s.evidenceUnavailable(w) {
		return
	}
	id := chi.URLParam(r, "id")
	result, err := s.evidenceProvider.VerifySession(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleEvidenceReport(w http.ResponseWriter, r *http.Request) {
	if s.evidenceUnavailable(w) {
		return
	}
	sessionID := chi.URLParam(r, "id")
	report, err := s.evidenceProvider.RenderReport(sessionID)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Write([]byte(report))
}

func (s *Server) handleEvidenceReportHTML(w http.ResponseWriter, r *http.Request) {
	if s.evidenceUnavailable(w) {
		return
	}
	sessionID := chi.URLParam(r, "id")
	report, err := s.evidenceProvider.RenderHTMLReport(sessionID)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(report))
}

// --- Credential handlers ---

func (s *Server) handleCredentialsList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.credentialProvider == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"credentials": []interface{}{}})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"credentials": s.credentialProvider.ActiveCredentials()})
}

func (s *Server) handleCredentialRevoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.credentialProvider == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "credential broker not enabled")
		return
	}
	if err := s.credentialProvider.RevokeCredential(id); err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// --- Approval handlers ---

func (s *Server) handleApprovalsPending(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.approvalProvider == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"pending": []interface{}{}})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"pending": s.approvalProvider.Pending()})
}

func (s *Server) handleApprovalsHistory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.approvalProvider == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"history": []interface{}{}})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"history": s.approvalProvider.History(100)})
}

func (s *Server) handleApprovalGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.approvalProvider == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	item, err := s.approvalProvider.Get(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

func (s *Server) handleApprovalApprove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.approvalProvider == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "approvals not enabled")
		return
	}
	var body struct {
		Reviewer string `json:"reviewer"`
		Comment  string `json:"comment"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Reviewer == "" {
		body.Reviewer = "admin"
	}
	item, err := s.approvalProvider.Approve(id, body.Reviewer, body.Comment)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

func (s *Server) handleApprovalDeny(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.approvalProvider == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "approvals not enabled")
		return
	}
	var body struct {
		Reviewer string `json:"reviewer"`
		Comment  string `json:"comment"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Reviewer == "" {
		body.Reviewer = "admin"
	}
	item, err := s.approvalProvider.Deny(id, body.Reviewer, body.Comment)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

// --- Test Action handler ---

type testActionRequest struct {
	Protocol   string            `json:"protocol"`
	Tool       string            `json:"tool"`
	Target     string            `json:"target"`
	Capability string            `json:"capability"`
	Params     map[string]string `json:"params"`
}

type testActionResponse struct {
	Decision              string      `json:"decision"`
	EnvelopeID            string      `json:"envelope_id"`
	EvidenceHash          string      `json:"evidence_hash"`
	Message               string      `json:"message"`
	ApprovalID            string      `json:"approval_id,omitempty"`
	CredentialProvenance  interface{} `json:"credential_provenance,omitempty"`
	DriftEvents           interface{} `json:"drift_events,omitempty"`
}

func (s *Server) handleTestAction(w http.ResponseWriter, r *http.Request) {
	var req testActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid JSON: " + err.Error())
		return
	}

	if req.Protocol == "" || req.Tool == "" || req.Target == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "protocol, tool, and target are required")
		return
	}

	// Default capability
	cap := envelope.Capability(req.Capability)
	if cap == "" {
		cap = envelope.CapExecute
	}

	// Create the action envelope
	actor := envelope.ActorInfo{
		Type:      "agent",
		ID:        "aegisctl-test",
		SessionID: "test-session",
		TenantID:  "test-tenant",
	}
	env := envelope.NewEnvelope(actor, "test-action", envelope.Protocol(req.Protocol), req.Tool, req.Target, cap)
	for k, v := range req.Params {
		env.Parameters[k] = v
	}

	// Evaluate via tool policy provider
	decision := "block"
	if s.toolPolicyProvider != nil {
		decision = s.toolPolicyProvider.Evaluate(env)
	}
	env.PolicyDecision = envelope.Decision(decision)
	env.EvidenceHash = env.Hash()

	resp := testActionResponse{
		Decision:     decision,
		EnvelopeID:   env.ID,
		EvidenceHash: env.EvidenceHash,
	}

	switch envelope.Decision(decision) {
	case envelope.DecisionAllow:
		resp.Message = "Action is allowed by policy"
		// Issue a credential and record provenance in the envelope parameters.
		if s.credentialProvider != nil {
			prov, err := s.credentialProvider.IssueCredential("static", "test-action", req.Target, string(cap), env.ID)
			if err == nil && prov != nil {
				env.Parameters["credential_provenance"] = prov
				resp.CredentialProvenance = prov
			}
		}
	case envelope.DecisionReview:
		resp.Message = "Action requires human review"
		if s.approvalProvider != nil {
			s.approvalProvider.Submit(env)
			resp.ApprovalID = env.ID
		}
	case envelope.DecisionBlock:
		resp.Message = "Action is blocked by policy"
	}

	// Run drift detection if a manifest exists for this task
	if s.manifestProvider != nil {
		driftEvents := s.manifestProvider.CheckDrift(env.Task, env, 1, 0.0)
		if driftEvents != nil {
			resp.DriftEvents = driftEvents
		}
	}

	// Record in audit log
	if s.auditProvider != nil {
		detail := fmt.Sprintf(`{"tool":"%s","target":"%s","capability":"%s","decision":"%s"}`, req.Tool, req.Target, req.Capability, decision)
		s.auditProvider.Log("agent", "agent", "test-action."+decision, "tool:"+req.Tool, detail, "test-tenant", "")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// --- Simulate handler (POST /admin/v1/simulate) ---

func (s *Server) handleSimulate(w http.ResponseWriter, r *http.Request) {
	var req testActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid JSON: " + err.Error())
		return
	}

	if req.Protocol == "" || req.Tool == "" || req.Target == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "protocol, tool, and target are required")
		return
	}

	cap := envelope.Capability(req.Capability)
	if cap == "" {
		cap = envelope.CapExecute
	}

	actor := envelope.ActorInfo{
		Type:      "agent",
		ID:        "simulate",
		SessionID: "simulate-session",
		TenantID:  "simulate-tenant",
	}
	env := envelope.NewEnvelope(actor, "simulate", envelope.Protocol(req.Protocol), req.Tool, req.Target, cap)

	result := map[string]interface{}{
		"action":   env.Tool,
		"decision": "block",
		"trace":    nil,
	}

	if s.toolPolicyProvider != nil {
		result["decision"] = s.toolPolicyProvider.Evaluate(env)
		result["trace"] = s.toolPolicyProvider.EvaluateWithTrace(env)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// --- Why handler (GET /admin/v1/actions/{id}/why) ---

// actionTraceStore is a simple in-memory store of recent decision traces keyed
// by envelope ID. In production this would be backed by a persistent store.
var actionTraceStore = make(map[string]interface{})

// RecordActionTrace stores a decision trace so it can be retrieved by the why
// endpoint.
func RecordActionTrace(id string, trace interface{}) {
	actionTraceStore[id] = trace
}

func (s *Server) handleActionWhy(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "action id is required")
		return
	}

	trace, ok := actionTraceStore[id]
	if !ok {
		writeAPIError(w, http.StatusNotFound, "not_found", "no trace found for action " + id)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trace)
}

// --- Manifest handlers ---

func (s *Server) handleManifestList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.manifestProvider == nil {
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}
	json.NewEncoder(w).Encode(s.manifestProvider.List())
}

func (s *Server) handleManifestGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	w.Header().Set("Content-Type", "application/json")
	if s.manifestProvider == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "manifest provider not available")
		return
	}
	m, err := s.manifestProvider.Get(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	json.NewEncoder(w).Encode(m)
}

func (s *Server) handleManifestDrift(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	w.Header().Set("Content-Type", "application/json")
	if s.manifestProvider == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "manifest provider not available")
		return
	}
	json.NewEncoder(w).Encode(s.manifestProvider.GetDrift(id))
}

type createManifestRequest struct {
	ID               string   `json:"id"`
	TaskID           string   `json:"task_id"`
	Description      string   `json:"description"`
	Owner            string   `json:"owner"`
	ExpiresIn        string   `json:"expires_in"`
	AllowedTools     []string `json:"allowed_tools"`
	AllowedResources []string `json:"allowed_resources"`
	AllowedProtocols []string `json:"allowed_protocols"`
	AllowedVerbs     []string `json:"allowed_verbs"`
	MaxActions       int      `json:"max_actions"`
	MaxBudget        float64  `json:"max_budget"`
	RiskTier         string   `json:"risk_tier"`
}

func (s *Server) handleManifestCreate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.manifestProvider == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "manifest provider not available")
		return
	}

	var req createManifestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "invalid JSON: " + err.Error())
		return
	}

	if req.TaskID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "task_id is required")
		return
	}

	id := req.ID
	if id == "" {
		id = fmt.Sprintf("manifest-%d", time.Now().UnixNano())
	}

	expiresAt := time.Now().UTC().Add(1 * time.Hour)
	if req.ExpiresIn != "" {
		dur, err := time.ParseDuration(req.ExpiresIn)
		if err == nil {
			expiresAt = time.Now().UTC().Add(dur)
		}
	}

	riskTier := req.RiskTier
	if riskTier == "" {
		riskTier = "medium"
	}

	m := map[string]interface{}{
		"id":                id,
		"task_id":           req.TaskID,
		"description":       req.Description,
		"owner":             req.Owner,
		"expires_at":        expiresAt,
		"allowed_tools":     req.AllowedTools,
		"allowed_resources": req.AllowedResources,
		"allowed_protocols": req.AllowedProtocols,
		"allowed_verbs":     req.AllowedVerbs,
		"max_actions":       req.MaxActions,
		"max_budget":        req.MaxBudget,
		"risk_tier":         riskTier,
	}

	if err := s.manifestProvider.Register(m); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	result, _ := s.manifestProvider.Get(id)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleManifestDeactivate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	w.Header().Set("Content-Type", "application/json")
	if s.manifestProvider == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "manifest provider not available")
		return
	}
	if err := s.manifestProvider.Deactivate(id); err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "deactivated"})
}

// --- Capability ticket handlers ---

func (s *Server) handleTicketsList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.capabilityProvider == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"tickets": []interface{}{}})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"tickets": s.capabilityProvider.ActiveTickets()})
}

func (s *Server) handleTicketRevoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.capabilityProvider == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "capability tickets not enabled")
		return
	}
	if err := s.capabilityProvider.RevokeTicket(id); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})
}

func (s *Server) handleTicketVerify(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if s.capabilityProvider == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "capability tickets not enabled")
		return
	}
	result, err := s.capabilityProvider.VerifyTicket(id)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleSupplyChain(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.supplyChainProvider == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled": false,
			"assets":  []interface{}{},
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"enabled": true,
		"assets":  s.supplyChainProvider.ListAssets(),
	})
}

func (s *Server) handleSessionRisk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	sessionID := chi.URLParam(r, "id")
	if sessionID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "session id required")
		return
	}
	if s.behavioralProvider == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled":    false,
			"session_id": sessionID,
			"risk_score": 0,
			"alerts":     []interface{}{},
		})
		return
	}
	result, err := s.behavioralProvider.SessionRisk(sessionID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	if result == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "session not found")
		return
	}
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleHealthDetailed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.resilienceProvider == nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "resilience not enabled"})
		return
	}
	json.NewEncoder(w).Encode(s.resilienceProvider.DetailedHealth())
}

func (s *Server) handleResilienceDegradation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.resilienceProvider == nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "resilience not enabled"})
		return
	}
	json.NewEncoder(w).Encode(s.resilienceProvider.DegradationModes())
}

func (s *Server) handleResilienceBackupCreate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.resilienceProvider == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "service_unavailable", "resilience not enabled")
		return
	}
	snap, err := s.resilienceProvider.CreateBackup()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(snap)
}

func (s *Server) handleResilienceBackupsList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.resilienceProvider == nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "resilience not enabled"})
		return
	}
	json.NewEncoder(w).Encode(s.resilienceProvider.ListBackups())
}

func (s *Server) handleResilienceRetention(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.resilienceProvider == nil {
		json.NewEncoder(w).Encode(map[string]string{"status": "resilience not enabled"})
		return
	}
	json.NewEncoder(w).Encode(s.resilienceProvider.RetentionStats())
}
