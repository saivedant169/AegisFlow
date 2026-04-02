package main

import (
	"bufio"
	"context"
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
	"github.com/saivedant169/AegisFlow/internal/audit"
	auditpg "github.com/saivedant169/AegisFlow/internal/audit/pgstore"
	"github.com/saivedant169/AegisFlow/internal/budget"
	"github.com/saivedant169/AegisFlow/internal/cache"
	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/eval"
	"github.com/saivedant169/AegisFlow/internal/federation"
	"github.com/saivedant169/AegisFlow/internal/gateway"
	"github.com/saivedant169/AegisFlow/internal/logger"
	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/policy"
	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/ratelimit"
	"github.com/saivedant169/AegisFlow/internal/rollout"
	rolloutpg "github.com/saivedant169/AegisFlow/internal/rollout/pgstore"
	"github.com/saivedant169/AegisFlow/internal/router"
	"github.com/saivedant169/AegisFlow/internal/storage"
	"github.com/saivedant169/AegisFlow/internal/telemetry"
	"github.com/saivedant169/AegisFlow/internal/usage"
	"github.com/saivedant169/AegisFlow/internal/webhook"
)

const version = "v0.4.0"

var totalRequests uint64

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

	// Config hot-reload
	watcher := config.NewWatcher(*configPath, cfg, func(newCfg *config.Config) {
		log.Printf("config reloaded — some changes require restart")
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
	wh := webhook.NewNotifier(cfg.Webhook.URL)
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
	handler := gateway.NewHandler(registry, rt, pe, ut, responseCache, wh, pgStore, analyticsCollector, cfg.Server.MaxBodySize, recordSpendFn, budgetCheckFn)

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
	if budgetMgr != nil {
		r.Use(middleware.BudgetCheck(budgetMgr.CheckFunc()))
	}
	r.Use(middleware.Logging)
	r.Use(middleware.Metrics)

	r.Get("/health", healthHandler)
	r.Post("/v1/chat/completions", handler.ChatCompletion)
	r.Get("/v1/models", handler.ListModels)

	// Request log for live feed
	reqLog := admin.NewRequestLog(200)

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
			dp := federation.NewDataPlane(cfg.Federation.ControlPlane.URL, cfg.Federation.ControlPlane.Token, cfg.Federation.ControlPlane.SyncInterval)
			dp.Start()
			defer dp.Stop()
			log.Printf("federation data plane enabled (control: %s)", cfg.Federation.ControlPlane.URL)
		}
	}

	// Admin server
	adminSvr := admin.NewServer(ut, cfg, registry, reqLog, responseCache, rolloutAdapter, analyticsAdapter, budgetAdapter, auditAdapter, federationProvider)

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
			registry.Register(provider.NewOpenAIProvider(pc.Name, pc.BaseURL, pc.APIKeyEnv, pc.Models, pc.Timeout, pc.MaxRetries))
			log.Printf("registered provider: %s (type: openai, base_url: %s)", pc.Name, pc.BaseURL)
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
			registry.Register(provider.NewAzureOpenAIProvider(pc.Name, pc.BaseURL, pc.APIKeyEnv, pc.APIVersion, pc.Models, pc.Timeout))
			log.Printf("registered provider: %s (type: azure_openai, endpoint: %s)", pc.Name, pc.BaseURL)
		case "groq":
			registry.Register(provider.NewGroqProvider(pc.Name, pc.APIKeyEnv, pc.Models, pc.Timeout))
			log.Printf("registered provider: %s (type: groq)", pc.Name)
		case "mistral":
			registry.Register(provider.NewMistralProvider(pc.Name, pc.APIKeyEnv, pc.Models, pc.Timeout))
			log.Printf("registered provider: %s (type: mistral)", pc.Name)
		case "together":
			registry.Register(provider.NewTogetherProvider(pc.Name, pc.APIKeyEnv, pc.Models, pc.Timeout))
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
	return policy.NewEngine(inputFilters, outputFilters)
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
