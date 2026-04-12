package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/saivedant169/AegisFlow/internal/admin"
	"github.com/saivedant169/AegisFlow/internal/analytics"
	"github.com/saivedant169/AegisFlow/internal/behavioral"
	"github.com/saivedant169/AegisFlow/internal/audit"
	auditpg "github.com/saivedant169/AegisFlow/internal/audit/pgstore"
	"github.com/saivedant169/AegisFlow/internal/budget"
	"github.com/saivedant169/AegisFlow/internal/costopt"
	"github.com/saivedant169/AegisFlow/internal/cache"
	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/eval"
	"github.com/saivedant169/AegisFlow/internal/federation"
	"github.com/saivedant169/AegisFlow/internal/gateway"
	"github.com/saivedant169/AegisFlow/internal/loadshed"
	"github.com/saivedant169/AegisFlow/internal/logger"
	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/policy"
	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/resilience"
	"github.com/saivedant169/AegisFlow/internal/ratelimit"
	"github.com/saivedant169/AegisFlow/internal/rollout"
	rolloutpg "github.com/saivedant169/AegisFlow/internal/rollout/pgstore"
	"github.com/saivedant169/AegisFlow/internal/router"
	"github.com/saivedant169/AegisFlow/internal/storage"
	"github.com/saivedant169/AegisFlow/internal/telemetry"
	"github.com/saivedant169/AegisFlow/internal/approval"
	approvalint "github.com/saivedant169/AegisFlow/internal/approval/integrations"
	"github.com/saivedant169/AegisFlow/internal/capability"
	"github.com/saivedant169/AegisFlow/internal/credential"
	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/usage"
	"github.com/saivedant169/AegisFlow/internal/manifest"
	"github.com/saivedant169/AegisFlow/internal/mcpgw"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
	"github.com/saivedant169/AegisFlow/internal/webhook"
)

const version = "v0.5.0"

var totalRequests uint64

// notifierAdapter bridges approvalint.ApprovalNotifier to approval.Notifier.
type notifierAdapter struct {
	inner approvalint.ApprovalNotifier
}

func (a *notifierAdapter) NotifyReview(item *approval.ApprovalItem) error {
	return a.inner.NotifyReview(item)
}
func (a *notifierAdapter) NotifyApproved(item *approval.ApprovalItem) error {
	return a.inner.NotifyApproved(item)
}
func (a *notifierAdapter) NotifyDenied(item *approval.ApprovalItem) error {
	return a.inner.NotifyDenied(item)
}

func main() {
	configPath := flag.String("config", "configs/aegisflow.yaml", "path to config file")
	showVersion := flag.Bool("version", false, "print aegisflow version")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	loadEnvFile(".env")

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Structured logger
	logger.Init(cfg.Logging.Level, cfg.Logging.Format)
	defer logger.Sync()

	// Config hot-reload (tpEngine and policyVersionStore are set below but
	// captured by reference, so the callback sees the final values).
	var tpEngineForWatcher **toolpolicy.Engine = new(*toolpolicy.Engine)
	var pvStoreForWatcher **toolpolicy.PolicyVersionStore = new(*toolpolicy.PolicyVersionStore)
	watcher := config.NewWatcher(*configPath, cfg, func(newCfg *config.Config) {
		log.Printf("config reloaded — some changes require restart")

		// Reload tool policies dynamically.
		if *tpEngineForWatcher != nil && newCfg.ToolPolicies.Enabled {
			newRules := make([]toolpolicy.ToolRule, len(newCfg.ToolPolicies.Rules))
			for i, r := range newCfg.ToolPolicies.Rules {
				newRules[i] = toolpolicy.ToolRule{
					Protocol:   r.Protocol,
					Tool:       r.Tool,
					Target:     r.Target,
					Capability: r.Capability,
					Decision:   r.Decision,
				}
			}
			(*tpEngineForWatcher).ReplaceRules(newRules, newCfg.ToolPolicies.DefaultDecision)
			if *pvStoreForWatcher != nil {
				(*pvStoreForWatcher).Snapshot(newRules, newCfg.ToolPolicies.DefaultDecision, "reload")
			}
			log.Printf("[config] tool policies reloaded (%d rules, default=%s)", len(newRules), newCfg.ToolPolicies.DefaultDecision)
		}
	})
	watcher.Start(5 * time.Second)
	defer watcher.Stop()

	// Telemetry
	if cfg.Telemetry.Enabled {
		shutdown, err := telemetry.Init("aegisflow", cfg.Telemetry.Exporter)
		if err != nil {
			log.Printf("telemetry init failed: %v", err)
		} else {
			defer shutdown()
		}
	}

	registry := provider.NewRegistry()
	initProviders(cfg, registry)

	rt := router.NewRouter(cfg.Routes, registry)
	pe := initPolicyEngine(cfg)
	usageStore := usage.NewStore()
	ut := usage.NewTracker(usageStore)

	// Response cache
	var responseCache cache.Cache
	if cfg.Cache.Enabled {
		responseCache = cache.NewMemoryCache(cfg.Cache.TTL, cfg.Cache.MaxSize)
		log.Printf("response cache enabled (backend: %s, ttl: %s, max_size: %d)", cfg.Cache.Backend, cfg.Cache.TTL, cfg.Cache.MaxSize)
	}

	// Semantic cache
	var semanticCache *cache.SemanticCache
	if cfg.Cache.Semantic.Enabled {
		var embedder cache.Embedder
		baseURL := cfg.Cache.Semantic.BaseURL
		if baseURL == "" {
			baseURL = "https://api.openai.com"
		}
		embedder = cache.NewOpenAIEmbedder(
			baseURL,
			cfg.Cache.Semantic.APIKey,
			cfg.Cache.Semantic.Model,
		)
		semanticCache = cache.NewSemanticCache(
			embedder,
			cfg.Cache.Semantic.Threshold,
			cfg.Cache.Semantic.MaxSize,
		)
		log.Printf("[init] semantic cache enabled (threshold=%.2f, max=%d)",
			cfg.Cache.Semantic.Threshold, cfg.Cache.Semantic.MaxSize)
	}

	// PostgreSQL persistent storage
	var pgStore *storage.PostgresStore
	if cfg.Database.Enabled {
		var err error
		pgStore, err = storage.NewPostgresStore(cfg.Database.ConnString)
		if err != nil {
			log.Printf("database connection failed (continuing without persistence): %v", err)
		} else {
			defer pgStore.Close()
			if err := pgStore.MigrateAudit(); err != nil {
				log.Printf("audit table migration failed: %v", err)
			}
			log.Printf("database connected: persistent usage storage and audit logging enabled")
		}
	}

	// Webhook notifier
	wh := webhook.NewNotifier(cfg.Webhook.URL, cfg.Webhook.Secret)
	if wh != nil {
		log.Printf("webhook notifications enabled: %s", cfg.Webhook.URL)
	}

	// Analytics
	var analyticsCollector *analytics.Collector
	var alertMgr *analytics.AlertManager
	if cfg.Analytics.Enabled {
		analyticsCollector = analytics.NewCollector(cfg.Analytics.RetentionHours)

		if cfg.Analytics.AnomalyDetection.Enabled {
			alertMgr = analytics.NewAlertManager(wh)
			detector := analytics.NewDetector(analyticsCollector,
				analytics.StaticThresholds{
					ErrorRateMax:         cfg.Analytics.AnomalyDetection.Static.ErrorRateMax,
					P95LatencyMax:        cfg.Analytics.AnomalyDetection.Static.P95LatencyMax,
					RequestsPerMinuteMax: cfg.Analytics.AnomalyDetection.Static.RequestsPerMinuteMax,
					CostPerMinuteMax:     cfg.Analytics.AnomalyDetection.Static.CostPerMinuteMax,
				},
				analytics.BaselineConfig{
					Window:          cfg.Analytics.AnomalyDetection.Baseline.Window,
					StddevThreshold: cfg.Analytics.AnomalyDetection.Baseline.StddevThreshold,
				},
			)
			detectorStop := make(chan struct{})
			go func() {
				ticker := time.NewTicker(cfg.Analytics.AnomalyDetection.EvaluationInterval)
				defer ticker.Stop()
				for {
					select {
					case <-detectorStop:
						return
					case <-ticker.C:
						result := detector.Evaluate()
						alertMgr.ProcessAlerts(result)
					}
				}
			}()
			defer close(detectorStop)
			log.Printf("anomaly detection enabled (interval: %s)", cfg.Analytics.AnomalyDetection.EvaluationInterval)
		}
		log.Printf("analytics enabled (retention: %dh)", cfg.Analytics.RetentionHours)
	}

	// Budget manager
	var budgetMgr *budget.Manager
	var budgetAdapter admin.BudgetProvider
	var budgetScopes []budget.SpendScope
	if cfg.Budgets.Enabled {
		// Build scopes from config
		if cfg.Budgets.Global.Monthly > 0 {
			budgetScopes = append(budgetScopes, budget.SpendScope{
				Scope: "global", ScopeID: "global",
				Limit:   cfg.Budgets.Global.Monthly,
				AlertAt: cfg.Budgets.Global.AlertAt, WarnAt: cfg.Budgets.Global.WarnAt,
			})
		}
		for tenantID, tb := range cfg.Budgets.Tenants {
			if tb.Monthly > 0 {
				budgetScopes = append(budgetScopes, budget.SpendScope{
					Scope: "tenant", ScopeID: tenantID,
					Limit:   tb.Monthly,
					AlertAt: tb.AlertAt, WarnAt: tb.WarnAt,
				})
			}
			for model, mb := range tb.Models {
				if mb.Monthly > 0 {
					budgetScopes = append(budgetScopes, budget.SpendScope{
						Scope: "tenant_model", ScopeID: tenantID + ":" + model,
						Limit:   mb.Monthly,
						AlertAt: mb.AlertAt, WarnAt: mb.WarnAt,
					})
				}
			}
		}
		budgetMgr = budget.NewManager(budgetScopes)
		budgetAdapter = budget.NewAdminAdapter(budgetMgr, budgetScopes)
		log.Printf("budget manager enabled (%d scopes)", len(budgetScopes))
	}

	var recordSpendFn func(string, string, float64)
	var budgetCheckFn func(string, string) (bool, []string, string)
	if budgetMgr != nil {
		recordSpendFn = budgetMgr.RecordSpend
		budgetCheckFn = budgetMgr.CheckFunc()
	}

	// Behavioral analysis engine
	var behavioralRegistry *behavioral.Registry
	if cfg.Behavioral.Enabled {
		behavioralRegistry = behavioral.NewRegistry(
			behavioral.DefaultRules(),
			cfg.Behavioral.KillSwitchScore,
			cfg.Behavioral.WindowMinutes,
		)
		log.Printf("[init] behavioral analysis enabled (kill_switch=%d, window=%dm)",
			cfg.Behavioral.KillSwitchScore, cfg.Behavioral.WindowMinutes)
	}

	// Request log for live feed
	reqLog := admin.NewRequestLog(200)

	handler := gateway.NewHandler(registry, rt, pe, ut, responseCache, wh, pgStore, analyticsCollector, cfg.Server.MaxBodySize, recordSpendFn, budgetCheckFn)
	handler.SetRequestLogger(reqLog, cfg.Federation.ControlPlane.Name)
	if semanticCache != nil {
		handler.SetSemanticCache(semanticCache)
	}
	if behavioralRegistry != nil {
		handler.SetBehavioralRegistry(behavioralRegistry)
	}

	// Eval hooks
	if cfg.Eval.Enabled {
		var webhookEval *eval.WebhookEvaluator
		if cfg.Eval.Webhook.URL != "" {
			webhookEval = eval.NewWebhookEvaluator(
				cfg.Eval.Webhook.URL, cfg.Eval.Webhook.SampleRate,
				cfg.Eval.Webhook.Timeout, cfg.Eval.Webhook.SendFullContent,
			)
		}
		handler.SetEval(cfg.Eval.Builtin.Enabled, cfg.Eval.Builtin.MinResponseTokens,
			cfg.Eval.Builtin.LatencyMultiplier, webhookEval)
		log.Printf("eval hooks enabled (builtin: %v, webhook: %v)", cfg.Eval.Builtin.Enabled, cfg.Eval.Webhook.URL != "")
	}

	// Set up transformations
	handler.SetTransformConfig(&gateway.TransformConfig{
		SystemPromptPrefix:  cfg.Transform.SystemPromptPrefix,
		SystemPromptSuffix:  cfg.Transform.SystemPromptSuffix,
		DefaultSystemPrompt: cfg.Transform.DefaultSystemPrompt,
	})

	if cfg.Transform.Response.StripPII || cfg.Transform.Response.ContentPrefix != "" ||
		cfg.Transform.Response.ContentSuffix != "" || len(cfg.Transform.Response.Replacements) > 0 {
		handler.SetResponseTransformConfig(&gateway.ResponseTransformConfig{
			StripPII:      cfg.Transform.Response.StripPII,
			ContentPrefix: cfg.Transform.Response.ContentPrefix,
			ContentSuffix: cfg.Transform.Response.ContentSuffix,
			Replacements:  cfg.Transform.Response.Replacements,
		})
	}

	if len(cfg.Aliases.Models) > 0 {
		handler.SetModelAliases(cfg.Aliases.Models)
	}

	// Build per-tenant transforms
	tenantTransforms := make(map[string]*gateway.TransformConfig)
	for _, t := range cfg.Tenants {
		if t.Transform != nil {
			tenantTransforms[t.ID] = &gateway.TransformConfig{
				SystemPromptPrefix:  t.Transform.SystemPromptPrefix,
				SystemPromptSuffix:  t.Transform.SystemPromptSuffix,
				DefaultSystemPrompt: t.Transform.DefaultSystemPrompt,
			}
		}
	}
	if len(tenantTransforms) > 0 {
		handler.SetTenantTransforms(tenantTransforms)
	}

	// Rate limiter
	// Use the highest tenant rate limit as the global limiter cap
	maxRPM := 60
	for _, t := range cfg.Tenants {
		if t.RateLimit.RequestsPerMinute > maxRPM {
			maxRPM = t.RateLimit.RequestsPerMinute
		}
	}
	limiter := ratelimit.NewMemoryLimiter(maxRPM, time.Minute)

	// Token rate limiter
	maxTPM := 100000
	for _, t := range cfg.Tenants {
		if t.RateLimit.TokensPerMinute > maxTPM {
			maxTPM = t.RateLimit.TokensPerMinute
		}
	}
	tokenLimiter := ratelimit.NewMemoryLimiter(maxTPM, time.Minute)

	// Gateway router
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddUint64(&totalRequests, 1)
			next.ServeHTTP(w, r)
		})
	})
	r.Use(chimw.RequestID)
	// RealIP trusts X-Forwarded-For/X-Real-IP headers.
	// In production, ensure only your reverse proxy (nginx, ALB, etc.) sets these.
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(middleware.CORS(cfg))
	r.Use(middleware.Auth(cfg))
	r.Use(middleware.RateLimit(limiter))
	r.Use(middleware.TokenRateLimit(tokenLimiter))
	if cfg.LoadShed.Enabled {
		shedder := loadshed.New(loadshed.Config{
			MaxConcurrent: cfg.LoadShed.MaxConcurrent,
			QueueSize:     cfg.LoadShed.QueueSize,
			QueueTimeout:  cfg.LoadShed.QueueTimeout,
		})
		r.Use(middleware.LoadShed(shedder))
		log.Printf("load shedding enabled (max_concurrent: %d, queue_size: %d, queue_timeout: %s)",
			cfg.LoadShed.MaxConcurrent, cfg.LoadShed.QueueSize, cfg.LoadShed.QueueTimeout)
	}
	if budgetMgr != nil {
		r.Use(middleware.BudgetCheck(budgetMgr.CheckFunc()))
	}
	r.Use(middleware.Logging)
	r.Use(middleware.Metrics)

	r.Get("/health", healthHandler)
	r.Post("/v1/chat/completions", handler.ChatCompletion)
	r.Get("/v1/models", handler.ListModels)

	// WebSocket endpoint
	if cfg.WebSocket.Enabled {
		wsHandler := handler.WebSocket(cfg, gateway.WebSocketConfig{
			Enabled:      true,
			PingInterval: cfg.WebSocket.PingInterval,
		})
		r.Get("/v1/ws", wsHandler.ServeHTTP)
		log.Printf("websocket endpoint enabled at /v1/ws (ping_interval: %s)", cfg.WebSocket.PingInterval)
	}

	// Rollout manager
	var rolloutAdapter admin.RolloutManager
	var rolloutStore rollout.Store = rollout.NewMemoryStore()
	if pgStore != nil {
		rolloutStore = rolloutpg.NewPostgresStore(pgStore.DB())
	}
	rolloutMgr, err := rollout.NewManager(rolloutStore, reqLog)
	if err != nil {
		log.Printf("rollout manager init failed: %v", err)
	} else {
		rt.SetRolloutManager(rolloutMgr)
		rolloutAdapter = rollout.NewAdminAdapter(rolloutMgr)
		rolloutMgr.Start()
		defer rolloutMgr.Stop()
		log.Printf("rollout manager started")
	}

	// Audit logger
	var auditLogger *audit.Logger
	if pgStore != nil {
		auditStore := auditpg.NewPostgresStore(pgStore.DB())
		var err error
		auditLogger, err = audit.NewLogger(auditStore)
		if err != nil {
			log.Printf("audit logger init failed: %v", err)
		} else {
			defer auditLogger.Stop()
			log.Printf("audit logger enabled")
		}
	} else {
		memStore := audit.NewMemoryStore()
		var err error
		auditLogger, err = audit.NewLogger(memStore)
		if err != nil {
			log.Printf("audit logger init failed: %v", err)
		} else {
			defer auditLogger.Stop()
			log.Printf("audit logger enabled (in-memory)")
		}
	}

	// Audit admin adapter
	var auditAdapter admin.AuditProvider
	if auditLogger != nil {
		auditAdapter = audit.NewAdminAdapter(auditLogger)
		handler.SetAuditLogger(auditLogger.Log)
	}

	// Analytics admin adapter
	var analyticsAdapter admin.AnalyticsProvider
	if analyticsCollector != nil && alertMgr != nil {
		analyticsAdapter = analytics.NewAdminAdapter(analyticsCollector, alertMgr)
	}

	// Federation
	var federationProvider admin.FederationProvider
	if cfg.Federation.Enabled {
		if cfg.Federation.Mode == "control-plane" {
			cp := federation.NewControlPlane(cfg)
			federationProvider = cp
			log.Printf("federation control plane enabled (%d data planes)", len(cfg.Federation.DataPlanes))
		} else if cfg.Federation.Mode == "data-plane" {
			dp := federation.NewDataPlane(cfg.Federation.ControlPlane.Name, cfg.Federation.ControlPlane.URL, cfg.Federation.ControlPlane.Token, cfg.Federation.ControlPlane.SyncInterval)
			dp.Start()
			defer dp.Stop()
			log.Printf("federation data plane enabled (name: %s, control: %s)", cfg.Federation.ControlPlane.Name, cfg.Federation.ControlPlane.URL)
		}
	}

	// Cost optimization engine
	var costOptAdapter admin.CostOptProvider
	if cfg.CostOpt.Enabled {
		costEngine := costopt.NewEngine(
			costopt.DefaultCostRegistry(),
			cfg.CostOpt.MinQualityTolerance,
		)
		usageFn := func() []costopt.UsageSnapshot {
			allUsage := ut.GetAllUsage()
			var snaps []costopt.UsageSnapshot
			for tenantID, tu := range allUsage {
				for _, pm := range tu.ByProviderModel {
					snaps = append(snaps, costopt.UsageSnapshot{
						TenantID:     tenantID,
						Model:        pm.Model,
						Provider:     pm.Provider,
						RequestCount: int(pm.Requests),
						TotalTokens:  pm.TotalTokens,
						TotalCost:    pm.EstimatedCostUSD,
					})
				}
			}
			return snaps
		}
		costOptAdapter = costopt.NewAdminAdapter(costEngine, usageFn)
		log.Printf("[init] cost optimization engine enabled")
	}

	// Evidence chain
	evidenceChain := evidence.NewSessionChain("aegisflow-main")
	evidenceAdapter := evidence.NewAdminAdapter(evidenceChain)
	log.Printf("[init] evidence chain enabled (session: %s)", evidenceChain.SessionID())

	// Approval queue
	approvalQueue := approval.NewQueue(1000)
	if cfg.ApprovalIntegrations.Timeout > 0 {
		approvalQueue.Timeout = cfg.ApprovalIntegrations.Timeout
	}
	approvalAdapter := approval.NewAdminAdapter(approvalQueue)
	log.Printf("[init] approval queue enabled (max_size=1000, timeout=%s)", approvalQueue.Timeout)

	// Approval notifiers (GitHub and Slack)
	var approvalNotifiers []approvalint.ApprovalNotifier
	if cfg.ApprovalIntegrations.GitHub.Enabled {
		ghNotifier := approvalint.NewGitHubNotifier(
			cfg.ApprovalIntegrations.GitHub.Token,
			"https://api.github.com",
			cfg.ApprovalIntegrations.GitHub.Repo,
		)
		approvalNotifiers = append(approvalNotifiers, ghNotifier)
		log.Printf("[init] GitHub approval notifier enabled (repo: %s)", cfg.ApprovalIntegrations.GitHub.Repo)
	}
	if cfg.ApprovalIntegrations.Slack.Enabled {
		adminURL := fmt.Sprintf("http://%s:%d", cfg.Server.Host, cfg.Server.AdminPort)
		slackNotifier := approvalint.NewSlackNotifier(
			cfg.ApprovalIntegrations.Slack.WebhookURL,
			adminURL,
		)
		approvalNotifiers = append(approvalNotifiers, slackNotifier)
		log.Printf("[init] Slack approval notifier enabled")
	}
	for _, n := range approvalNotifiers {
		approvalQueue.AddNotifier(&notifierAdapter{inner: n})
	}

	// Approval timeout cleanup goroutine
	approvalCleanupStop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-approvalCleanupStop:
				return
			case <-ticker.C:
				if n := approvalQueue.CleanupExpired(); n > 0 {
					log.Printf("[approval] expired %d timed-out approval items", n)
				}
			}
		}
	}()
	defer close(approvalCleanupStop)

	// Credential broker
	var credentialAdapter admin.CredentialProvider
	if cfg.Credentials.Enabled {
		credRegistry := credential.NewRegistry()
		for _, pc := range cfg.Credentials.Providers {
			switch pc.Type {
			case "static":
				ttl := pc.DefaultTTL
				if ttl == 0 {
					ttl = 1 * time.Hour
				}
				broker := credential.NewStaticBroker(pc.Name, pc.Token, ttl)
				credRegistry.Register(pc.Name, broker)
				log.Printf("[init] registered static credential broker: %s", pc.Name)
			case "github_app":
				ttl := pc.DefaultTTL
				if ttl == 0 {
					ttl = 1 * time.Hour
				}
				broker := credential.NewGitHubAppBroker(pc.Name, pc.GitHubAppID, pc.GitHubKeyPath, pc.GitHubInstallID, ttl)
				credRegistry.Register(pc.Name, broker)
				log.Printf("[init] registered GitHub App credential broker: %s (app_id: %d, install_id: %d)", pc.Name, pc.GitHubAppID, pc.GitHubInstallID)
			case "vault":
				ttl := pc.DefaultTTL
				if ttl == 0 {
					ttl = 30 * time.Minute
				}
				broker := credential.NewVaultBroker(pc.Name, pc.VaultAddr, pc.VaultToken, pc.VaultSecretPath, ttl, nil)
				credRegistry.Register(pc.Name, broker)
				log.Printf("[init] registered Vault credential broker: %s (addr: %s, path: %s)", pc.Name, pc.VaultAddr, pc.VaultSecretPath)
			case "aws_sts":
				ttl := pc.DefaultTTL
				if ttl == 0 {
					ttl = 1 * time.Hour
				}
				region := pc.AWSRegion
				if region == "" {
					region = "us-east-1"
				}
				stsClient := credential.NewHTTPSTSClient(
					os.Getenv("AWS_ACCESS_KEY_ID"),
					os.Getenv("AWS_SECRET_ACCESS_KEY"),
					region,
				)
				broker := credential.NewAWSSTSBroker(pc.Name, credential.AWSSTSBrokerConfig{
					RoleARN:           pc.AWSRoleARN,
					Region:            region,
					SessionNamePrefix: "aegisflow",
					ExternalID:        pc.AWSExternalID,
					DefaultTTL:        ttl,
				}, stsClient)
				credRegistry.Register(pc.Name, broker)
				log.Printf("[init] registered AWS STS credential broker: %s (role: %s, region: %s)", pc.Name, pc.AWSRoleARN, region)
			default:
				log.Printf("[init] skipping unsupported credential provider type: %s", pc.Type)
			}
		}
		credentialAdapter = credential.NewAdminAdapter(credRegistry)
		// Start periodic cleanup of expired credentials.
		credCleanupStop := make(chan struct{})
		go func() {
			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-credCleanupStop:
					return
				case <-ticker.C:
					credRegistry.CleanupExpired()
				}
			}
		}()
		defer close(credCleanupStop)
		log.Printf("[init] credential broker enabled (%d providers)", len(cfg.Credentials.Providers))
	}

	// Tool policy engine for admin test-action endpoint
	var toolPolicyOpt admin.ServerOption
	var tpEngine *toolpolicy.Engine
	policyVersionStore := toolpolicy.NewPolicyVersionStore(20)
	{
		tpEngine = toolpolicy.NewEngine(nil, cfg.ToolPolicies.DefaultDecision)
		if cfg.ToolPolicies.Enabled {
			rules := make([]toolpolicy.ToolRule, len(cfg.ToolPolicies.Rules))
			for i, r := range cfg.ToolPolicies.Rules {
				rules[i] = toolpolicy.ToolRule{
					Protocol:   r.Protocol,
					Tool:       r.Tool,
					Target:     r.Target,
					Capability: r.Capability,
					Decision:   r.Decision,
				}
			}
			tpEngine = toolpolicy.NewEngine(rules, cfg.ToolPolicies.DefaultDecision)
		}
		policyVersionStore.Snapshot(tpEngine.Rules(), tpEngine.DefaultDecision(), "initial")
		toolPolicyOpt = admin.WithToolPolicyProvider(toolpolicy.NewAdminAdapter(tpEngine))
		*tpEngineForWatcher = tpEngine
		*pvStoreForWatcher = policyVersionStore
	}

	// Manifest store and drift detector
	manifestStore := manifest.NewStore()
	manifestDetector := manifest.NewDriftDetector()
	manifestAdapter := manifest.NewAdminAdapter(manifestStore, manifestDetector)
	manifestOpt := admin.WithManifestProvider(manifestAdapter)
	log.Printf("[init] manifest drift detection enabled")

	// Capability ticket issuer
	var capabilityOpt admin.ServerOption
	if cfg.Capability.Enabled {
		signingKey, err := hex.DecodeString(cfg.Capability.SigningKey)
		if err != nil {
			log.Fatalf("invalid capability signing key (must be hex-encoded): %v", err)
		}
		capIssuer := capability.NewIssuer(signingKey)
		capabilityOpt = admin.WithCapabilityProvider(capability.NewAdminAdapter(capIssuer))
		// Periodic cleanup of expired tickets.
		capCleanupStop := make(chan struct{})
		go func() {
			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-capCleanupStop:
					return
				case <-ticker.C:
					capIssuer.Store().CleanupExpired()
				}
			}
		}()
		defer close(capCleanupStop)
		log.Printf("[init] capability tickets enabled (ttl: %s)", cfg.Capability.DefaultTTL)
	}

	// Resilience subsystem
	var resilienceOpt admin.ServerOption
	if cfg.Resilience.Enabled {
		healthReg := resilience.NewHealthRegistry()
		degradationMgr := resilience.NewDegradationManager()
		retentionMgr := resilience.NewRetentionManager(resilience.RetentionPolicy{
			AuditLogDays:        cfg.Resilience.Retention.AuditLogDays,
			EvidenceDays:        cfg.Resilience.Retention.EvidenceDays,
			ApprovalHistoryDays: cfg.Resilience.Retention.ApprovalHistoryDays,
			CompressAfterDays:   cfg.Resilience.Retention.CompressAfterDays,
			AutoCleanup:         cfg.Resilience.Retention.AutoCleanup,
		})
		backupMgr := resilience.NewBackupManager(cfg.Resilience.BackupDir)
		resAdapter := resilience.NewAdminAdapter(healthReg, degradationMgr, retentionMgr, backupMgr)
		resilienceOpt = admin.WithResilienceProvider(resAdapter)
		log.Printf("[init] resilience enabled (health_interval: %s, backup_dir: %s)", cfg.Resilience.HealthInterval, cfg.Resilience.BackupDir)
	}

	// Policy version adapter
	versionAdapter := toolpolicy.NewVersionAdminAdapter(policyVersionStore, tpEngine)
	policyVersionOpt := admin.WithPolicyVersionProvider(versionAdapter)

	// Admin server
	adminOpts := []admin.ServerOption{toolPolicyOpt, manifestOpt, policyVersionOpt}
	if capabilityOpt != nil {
		adminOpts = append(adminOpts, capabilityOpt)
	}
	if resilienceOpt != nil {
		adminOpts = append(adminOpts, resilienceOpt)
	}
	if behavioralRegistry != nil {
		adminOpts = append(adminOpts, admin.WithBehavioralProvider(behavioral.NewAdminAdapter(behavioralRegistry)))
	}
	adminSvr := admin.NewServer(ut, cfg, registry, reqLog, responseCache, rolloutAdapter, analyticsAdapter, budgetAdapter, auditAdapter, federationProvider, costOptAdapter, evidenceAdapter, approvalAdapter, credentialAdapter, adminOpts...)

	gatewayAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	adminAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.AdminPort)

	gatewaySrv := &http.Server{
		Addr:         gatewayAddr,
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	adminSrv := &http.Server{
		Addr:         adminAddr,
		Handler:      adminSvr.Router(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// MCP Gateway
	if cfg.MCPGateway.Enabled {
		upstreams := make([]mcpgw.UpstreamConfig, len(cfg.MCPGateway.Upstreams))
		for i, u := range cfg.MCPGateway.Upstreams {
			upstreams[i] = mcpgw.UpstreamConfig{
				Name:  u.Name,
				URL:   u.URL,
				Tools: u.Tools,
			}
		}
		toolPolicyEngine := toolpolicy.NewEngine(nil, cfg.ToolPolicies.DefaultDecision)
		if cfg.ToolPolicies.Enabled {
			rules := make([]toolpolicy.ToolRule, len(cfg.ToolPolicies.Rules))
			for i, r := range cfg.ToolPolicies.Rules {
				rules[i] = toolpolicy.ToolRule{
					Protocol:   r.Protocol,
					Tool:       r.Tool,
					Target:     r.Target,
					Capability: r.Capability,
					Decision:   r.Decision,
				}
			}
			toolPolicyEngine = toolpolicy.NewEngine(rules, cfg.ToolPolicies.DefaultDecision)
		}
		mcpGateway := mcpgw.NewGateway(toolPolicyEngine, nil, approvalQueue, upstreams)
		mcpAddr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.MCPGateway.Port)
		mcpSrv := &http.Server{
			Addr:         mcpAddr,
			Handler:      mcpGateway,
			ReadTimeout:  cfg.Server.ReadTimeout,
			WriteTimeout: cfg.Server.WriteTimeout,
		}
		go func() {
			log.Printf("MCP gateway listening on %s", mcpAddr)
			if err := mcpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("MCP gateway server error: %v", err)
			}
		}()
		defer mcpSrv.Shutdown(context.Background())
		log.Printf("[init] MCP gateway enabled (%d upstreams, port %d)", len(upstreams), cfg.MCPGateway.Port)
	}

	// Log initialization summary
	log.Printf("AegisFlow ready: %d providers, %d routes, %d tenants, %d policies", len(registry.List()), len(cfg.Routes), len(cfg.Tenants), len(cfg.Policies.Input)+len(cfg.Policies.Output))

	// Start servers
	go func() {
		log.Printf("AegisFlow gateway listening on %s", gatewayAddr)
		if err := gatewaySrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("gateway server error: %v", err)
		}
	}()

	go func() {
		log.Printf("AegisFlow admin API listening on %s", adminAddr)
		if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("admin server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down servers...")
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.GracefulShutdown)
	defer cancel()

	gatewaySrv.Shutdown(ctx)
	adminSrv.Shutdown(ctx)
	handler.Close()
	log.Println("servers stopped")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	n := atomic.LoadUint64(&totalRequests)
	fmt.Fprintf(w, `{"status":"ok","requests":%d}`, n)
}

// buildKeyRotator constructs a KeyRotator from a provider config.
// It supports the new api_keys list (with key or key_env per entry) as well as
// the legacy single api_key_env field for backward compatibility.
func buildKeyRotator(pc config.ProviderConfig) *provider.KeyRotator {
	var keys []string
	for _, entry := range pc.APIKeys {
		switch {
		case entry.Key != "":
			keys = append(keys, entry.Key)
		case entry.KeyEnv != "":
			if v := os.Getenv(entry.KeyEnv); v != "" {
				keys = append(keys, v)
			}
		}
	}
	// Fall back to the legacy single-key field if no api_keys were resolved.
	if len(keys) == 0 && pc.APIKeyEnv != "" {
		keys = append(keys, os.Getenv(pc.APIKeyEnv))
	}
	return provider.NewKeyRotator(keys, pc.KeySelection, 0)
}

func initProviders(cfg *config.Config, registry *provider.Registry) {
	for _, pc := range cfg.Providers {
		if !pc.Enabled {
			continue
		}

		switch pc.Type {
		case "mock":
			latency := 100 * time.Millisecond
			if v, ok := pc.Config["latency"]; ok {
				if d, err := time.ParseDuration(v); err == nil {
					latency = d
				}
			}
			registry.Register(provider.NewMockProvider(pc.Name, latency))
			log.Printf("registered provider: %s (type: mock, latency: %s)", pc.Name, latency)
		case "openai":
			kr := buildKeyRotator(pc)
			p := provider.NewOpenAIProviderWithKeys(pc.Name, pc.BaseURL, kr, pc.Models, pc.Timeout, pc.MaxRetries)
			p.ConfigureRetry(pc.Retry)
			registry.Register(p)
			log.Printf("registered provider: %s (type: openai, base_url: %s, keys: %d)", pc.Name, pc.BaseURL, kr.Len())
		case "anthropic":
			registry.Register(provider.NewAnthropicProvider(pc.Name, pc.BaseURL, pc.APIKeyEnv, pc.Models, pc.Timeout))
			log.Printf("registered provider: %s (type: anthropic, base_url: %s)", pc.Name, pc.BaseURL)
		case "ollama":
			registry.Register(provider.NewOllamaProvider(pc.Name, pc.BaseURL, pc.Models))
			log.Printf("registered provider: %s (type: ollama, base_url: %s)", pc.Name, pc.BaseURL)
		case "gemini":
			registry.Register(provider.NewGeminiProvider(pc.Name, pc.APIKeyEnv, pc.Models, pc.Timeout))
			log.Printf("registered provider: %s (type: gemini)", pc.Name)
		case "azure_openai":
			p := provider.NewAzureOpenAIProvider(pc.Name, pc.BaseURL, pc.APIKeyEnv, pc.APIVersion, pc.Models, pc.Timeout)
			p.ConfigureRetry(pc.Retry)
			registry.Register(p)
			log.Printf("registered provider: %s (type: azure_openai, endpoint: %s)", pc.Name, pc.BaseURL)
		case "groq":
			p := provider.NewGroqProvider(pc.Name, pc.APIKeyEnv, pc.Models, pc.Timeout)
			p.ConfigureRetry(pc.Retry)
			registry.Register(p)
			log.Printf("registered provider: %s (type: groq)", pc.Name)
		case "mistral":
			p := provider.NewMistralProvider(pc.Name, pc.APIKeyEnv, pc.Models, pc.Timeout)
			p.ConfigureRetry(pc.Retry)
			registry.Register(p)
			log.Printf("registered provider: %s (type: mistral)", pc.Name)
		case "together":
			p := provider.NewTogetherProvider(pc.Name, pc.APIKeyEnv, pc.Models, pc.Timeout)
			p.ConfigureRetry(pc.Retry)
			registry.Register(p)
			log.Printf("registered provider: %s (type: together)", pc.Name)
		case "bedrock":
			registry.Register(provider.NewBedrockProvider(pc.Name, pc.Config["region"], pc.APIKeyEnv, pc.Config["secret_key_env"], pc.Models, pc.Timeout))
			log.Printf("registered provider: %s (type: bedrock, region: %s)", pc.Name, pc.Config["region"])
		default:
			log.Printf("skipping unsupported provider type: %s", pc.Type)
		}
	}
}

func initPolicyEngine(cfg *config.Config) *policy.Engine {
	var inputFilters []policy.Filter
	for _, p := range cfg.Policies.Input {
		switch p.Type {
		case "keyword":
			inputFilters = append(inputFilters, policy.NewKeywordFilter(p.Name, policy.Action(p.Action), p.Keywords))
		case "regex":
			inputFilters = append(inputFilters, policy.NewRegexFilter(p.Name, policy.Action(p.Action), p.Patterns))
		case "pii":
			inputFilters = append(inputFilters, policy.NewPIIFilter(p.Name, policy.Action(p.Action), p.Patterns))
		case "wasm":
			timeout := p.Timeout
			if timeout == 0 {
				timeout = 100 * time.Millisecond
			}
			onError := p.OnError
			if onError == "" {
				onError = "block"
			}
			wf, err := policy.NewWasmFilter(p.Name, policy.Action(p.Action), p.Path, timeout, onError)
			if err != nil {
				log.Printf("failed to load wasm input filter %s: %v", p.Name, err)
				continue
			}
			log.Printf("loaded wasm input filter: %s (path: %s, timeout: %s, on_error: %s)", p.Name, p.Path, timeout, onError)
			inputFilters = append(inputFilters, wf)
		}
	}

	var outputFilters []policy.Filter
	for _, p := range cfg.Policies.Output {
		switch p.Type {
		case "keyword":
			outputFilters = append(outputFilters, policy.NewKeywordFilter(p.Name, policy.Action(p.Action), p.Keywords))
		case "regex":
			outputFilters = append(outputFilters, policy.NewRegexFilter(p.Name, policy.Action(p.Action), p.Patterns))
		case "wasm":
			timeout := p.Timeout
			if timeout == 0 {
				timeout = 100 * time.Millisecond
			}
			onError := p.OnError
			if onError == "" {
				onError = "block"
			}
			wf, err := policy.NewWasmFilter(p.Name, policy.Action(p.Action), p.Path, timeout, onError)
			if err != nil {
				log.Printf("failed to load wasm output filter %s: %v", p.Name, err)
				continue
			}
			log.Printf("loaded wasm output filter: %s (path: %s, timeout: %s, on_error: %s)", p.Name, p.Path, timeout, onError)
			outputFilters = append(outputFilters, wf)
		}
	}

	log.Printf("loaded %d input policies, %d output policies", len(inputFilters), len(outputFilters))

	var opts []policy.EngineOption
	if cfg.Policies.GovernanceMode != "" {
		opts = append(opts, policy.WithGovernanceMode(policy.GovernanceMode(cfg.Policies.GovernanceMode)))
	}
	if cfg.Policies.BreakGlass {
		opts = append(opts, policy.WithBreakGlass(true))
	}

	return policy.NewEngine(inputFilters, outputFilters, opts...)
}

func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if os.Getenv(key) == "" {
				os.Setenv(key, val)
			}
		}
	}
}
