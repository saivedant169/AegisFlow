package gateway

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"context"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/saivedant169/AegisFlow/internal/admin"
	"github.com/saivedant169/AegisFlow/internal/analytics"
	"github.com/saivedant169/AegisFlow/internal/behavioral"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/cache"
	"github.com/saivedant169/AegisFlow/internal/eval"
	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/policy"
	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/router"
	"github.com/saivedant169/AegisFlow/internal/storage"
	"github.com/saivedant169/AegisFlow/internal/usage"
	"github.com/saivedant169/AegisFlow/internal/webhook"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

type Handler struct {
	registry       *provider.Registry
	router         *router.Router
	policy         *policy.Engine
	usage          *usage.Tracker
	cache          cache.Cache
	webhook        *webhook.Notifier
	store          *storage.PostgresStore
	dbQueue        chan storage.UsageEvent
	analytics      *analytics.Collector
	maxBodySize    int64
	recordSpend    func(tenantID, model string, cost float64)
	budgetCheck    func(tenantID, model string) (bool, []string, string)
	evalBuiltin    bool
	evalMinTokens  int
	evalLatencyMul float64
	evalWebhook    *eval.WebhookEvaluator
	auditLog             func(actor, actorRole, action, resource, detail, tenantID, model string)
	requestLog           *admin.RequestLog
	dataPlaneName        string
	transformCfg         *TransformConfig
	responseTransformCfg *ResponseTransformConfig
	modelAliases         map[string]string
	tenantTransforms     map[string]*TransformConfig // tenant ID -> transform config
	semanticCache        *cache.SemanticCache
	behavioralRegistry   *behavioral.Registry
}

// SetAuditLogger sets the audit logging function on the handler.
func (h *Handler) SetAuditLogger(logFn func(actor, actorRole, action, resource, detail, tenantID, model string)) {
	h.auditLog = logFn
}

// SetRequestLogger configures live request feed logging on the handler.
func (h *Handler) SetRequestLogger(reqLog *admin.RequestLog, dataPlaneName string) {
	h.requestLog = reqLog
	h.dataPlaneName = dataPlaneName
}

// SetTransformConfig sets the global request transform configuration.
func (h *Handler) SetTransformConfig(cfg *TransformConfig) {
	h.transformCfg = cfg
}

// SetResponseTransformConfig sets the global response transform configuration.
func (h *Handler) SetResponseTransformConfig(cfg *ResponseTransformConfig) {
	h.responseTransformCfg = cfg
}

// SetModelAliases sets the model alias mapping for request rewriting.
func (h *Handler) SetModelAliases(aliases map[string]string) {
	h.modelAliases = aliases
}

// SetTenantTransforms sets per-tenant transform overrides.
func (h *Handler) SetTenantTransforms(transforms map[string]*TransformConfig) {
	h.tenantTransforms = transforms
}

// SetSemanticCache configures the semantic (embedding-based) cache on the handler.
func (h *Handler) SetSemanticCache(sc *cache.SemanticCache) {
	h.semanticCache = sc
}

// SetBehavioralRegistry configures the behavioral analysis registry on the handler.
func (h *Handler) SetBehavioralRegistry(reg *behavioral.Registry) {
	h.behavioralRegistry = reg
}

const (
	dbQueueSize        = 1024
	defaultMaxBodySize = 10 * 1024 * 1024
)

func NewHandler(registry *provider.Registry, rt *router.Router, pe *policy.Engine, ut *usage.Tracker, c cache.Cache, wh *webhook.Notifier, store *storage.PostgresStore, ac *analytics.Collector, maxBodySize int64, recordSpend func(string, string, float64), budgetCheck func(string, string) (bool, []string, string)) *Handler {
	if maxBodySize <= 0 {
		maxBodySize = defaultMaxBodySize
	}
	h := &Handler{registry: registry, router: rt, policy: pe, usage: ut, cache: c, webhook: wh, store: store, analytics: ac, maxBodySize: maxBodySize, recordSpend: recordSpend, budgetCheck: budgetCheck}
	if store != nil {
		h.dbQueue = make(chan storage.UsageEvent, dbQueueSize)
		go h.dbWorker()
	}
	return h
}

// dbWorker drains the queue and writes events to the database sequentially,
// preventing unbounded goroutine growth when the DB is slow.
func (h *Handler) dbWorker() {
	for event := range h.dbQueue {
		if err := h.store.RecordEvent(context.Background(), event); err != nil {
			log.Printf("db worker: failed to record event: %v", err)
		}
	}
}

// Close shuts down the handler's background workers cleanly.
func (h *Handler) Close() {
	if h.dbQueue != nil {
		close(h.dbQueue)
	}
}

// SetEval configures the quality evaluation hooks on the handler.
func (h *Handler) SetEval(builtinEnabled bool, minTokens int, latencyMul float64, webhook *eval.WebhookEvaluator) {
	h.evalBuiltin = builtinEnabled
	h.evalMinTokens = minTokens
	h.evalLatencyMul = latencyMul
	h.evalWebhook = webhook
}

func (h *Handler) ChatCompletion(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	var req types.ChatCompletionRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, h.maxBodySize)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "failed to parse request body")
		return
	}

	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "model is required")
		return
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "messages is required")
		return
	}

	tenantID := ""
	if t := middleware.TenantFromContext(r.Context()); t != nil {
		tenantID = t.ID
	}

	// Behavioral kill-switch check
	sessionID := tenantID
	if h.behavioralRegistry != nil && sessionID != "" {
		sa := h.behavioralRegistry.GetOrCreate(sessionID)
		if sa.Blocked() {
			writeError(w, http.StatusForbidden, "session_blocked", "session blocked by behavioral kill switch — cumulative risk score exceeded threshold")
			return
		}
	}

	// Apply model aliasing
	if h.modelAliases != nil {
		ApplyModelAlias(&req, h.modelAliases)
	}

	// Apply request transformations (per-tenant overrides global)
	var tenantTransform *TransformConfig
	if tenantID != "" && h.tenantTransforms != nil {
		tenantTransform = h.tenantTransforms[tenantID]
	}
	TransformRequestWithTenant(&req, h.transformCfg, tenantTransform)

	// Per-model budget check (global/tenant checks run in middleware, model-level here)
	if h.budgetCheck != nil && tenantID != "" {
		allowed, warnings, blockMsg := h.budgetCheck(tenantID, req.Model)
		if !allowed {
			h.recordAnalytics(tenantID, req.Model, "", http.StatusTooManyRequests, startTime, 0)
			writeError(w, http.StatusTooManyRequests, "budget_exceeded", blockMsg)
			return
		}
		for _, warning := range warnings {
			w.Header().Add("X-AegisFlow-Budget-Warning", warning)
		}
	}

	// Policy check: input
	if h.policy != nil {
		inputContent := extractContent(req.Messages)
		v, err := h.policy.CheckInput(inputContent)
		if err != nil {
			log.Printf("policy engine input check error: %v", err)
			h.recordAnalytics(tenantID, req.Model, "", http.StatusInternalServerError, startTime, 0)
			writeError(w, http.StatusInternalServerError, "policy_error", "policy engine error")
			return
		}
		if v != nil {
			if v.Action == policy.ActionBlock {
				h.fireWebhook("policy_violation", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
				h.recordAnalytics(tenantID, req.Model, "", http.StatusForbidden, startTime, 0)
				if h.auditLog != nil {
					prompt := inputContent
					if len(prompt) > 200 {
						prompt = prompt[:200]
					}
					h.auditLog("system", "system", "policy.block", "policy:"+v.PolicyName, `{"message":"`+v.Message+`","prompt":"`+prompt+`"}`, tenantID, req.Model)
				}
				writeError(w, http.StatusForbidden, "policy_violation", policy.FormatViolation(v))
				return
			}
			h.fireWebhook("policy_warning", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
			log.Printf("policy warning: %s", policy.FormatViolation(v))
		}
	}

	if req.Stream {
		h.handleStream(w, r, &req, tenantID)
		return
	}

	// Semantic cache lookup (non-streaming only)
	if h.semanticCache != nil {
		if cached, ok := h.semanticCache.GetSemantic(tenantID, &req); ok {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-AegisFlow-Cache", "SEMANTIC-HIT")
			h.logRequest(startTime, r, tenantID, req.Model, "semantic-cache", http.StatusOK, cached.Usage.TotalTokens, true, "")
			json.NewEncoder(w).Encode(cached)
			return
		}
	}

	// Check cache (non-streaming only)
	if h.cache != nil {
		cacheKey := cache.BuildKey(tenantID, req.Model, req.Messages)
		if cached, ok := h.cache.Get(cacheKey); ok {
			log.Printf("cache hit: %s", cacheKey[:20])
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-AegisFlow-Cache", "HIT")
			h.logRequest(startTime, r, tenantID, req.Model, "cache", http.StatusOK, cached.Usage.TotalTokens, true, "")
			json.NewEncoder(w).Encode(cached)
			return
		}
	}

	routed, err := h.router.RouteWithProvider(r.Context(), &req)
	if err != nil {
		h.recordAnalytics(tenantID, req.Model, "", http.StatusBadGateway, startTime, 0)
		h.logRequest(startTime, r, tenantID, req.Model, "", http.StatusBadGateway, 0, false, "")
		writeError(w, http.StatusBadGateway, "provider_error", err.Error())
		return
	}
	resp := routed.Response
	providerName := routed.Provider

	// Policy check: output
	if h.policy != nil && len(resp.Choices) > 0 {
		v, err := h.policy.CheckOutput(resp.Choices[0].Message.Content)
		if err != nil {
			log.Printf("policy engine output check error: %v", err)
			h.recordAnalytics(tenantID, req.Model, providerName, http.StatusInternalServerError, startTime, 0)
			writeError(w, http.StatusInternalServerError, "policy_error", "policy engine error")
			return
		}
		if v != nil {
			if v.Action == policy.ActionBlock {
				h.fireWebhook("policy_violation", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
				h.recordAnalytics(tenantID, req.Model, providerName, http.StatusForbidden, startTime, 0)
				h.logRequest(startTime, r, tenantID, req.Model, providerName, http.StatusForbidden, resp.Usage.TotalTokens, false, routed.Region)
				if h.auditLog != nil {
					h.auditLog("system", "system", "policy.block", "policy:"+v.PolicyName, `{"message":"`+v.Message+`","phase":"output"}`, tenantID, req.Model)
				}
				writeError(w, http.StatusForbidden, "policy_violation", policy.FormatViolation(v))
				return
			}
			log.Printf("policy warning (output): %s", policy.FormatViolation(v))
		}
	}

	// Apply response transformations
	if h.responseTransformCfg != nil {
		TransformResponse(resp, h.responseTransformCfg)
	}

	// Cache the response
	if h.cache != nil {
		cacheKey := cache.BuildKey(tenantID, req.Model, req.Messages)
		h.cache.Set(cacheKey, resp)
	}
	if h.semanticCache != nil {
		h.semanticCache.SetSemantic(tenantID, &req, resp)
	}

	// Track usage
	if h.usage != nil {
		h.usage.Record(tenantID, providerName, req.Model, resp.Usage)
	}

	// Persist to database via buffered worker queue (non-blocking)
	if h.dbQueue != nil {
		select {
		case h.dbQueue <- storage.UsageEvent{
			TenantID: tenantID, Model: req.Model,
			PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens: resp.Usage.TotalTokens, StatusCode: 200,
			LatencyMs: time.Since(startTime).Milliseconds(), CreatedAt: time.Now(),
		}:
		default:
			log.Printf("db queue full — dropping usage event for tenant %s", tenantID)
		}
	}

	// Record spend for budget tracking
	if h.recordSpend != nil {
		h.recordSpend(tenantID, req.Model, float64(resp.Usage.TotalTokens)*0.00001)
	}

	// Quality evaluation
	var qualityScore int
	if h.evalBuiltin {
		evalResult := eval.ScoreResponse(resp, time.Since(startTime).Milliseconds(), 0, h.evalMinTokens, h.evalLatencyMul)
		qualityScore = evalResult.Score
	}

	if h.evalWebhook != nil && h.evalWebhook.ShouldEvaluate() {
		prompt := ""
		if len(req.Messages) > 0 {
			prompt = req.Messages[len(req.Messages)-1].Content
		}
		response := ""
		if len(resp.Choices) > 0 {
			response = resp.Choices[0].Message.Content
		}
		h.evalWebhook.Evaluate(eval.WebhookRequest{
			Model: req.Model, Provider: providerName,
			Prompt: prompt, Response: response,
			LatencyMs:    time.Since(startTime).Milliseconds(),
			BuiltinScore: qualityScore,
		})
	}

	// Behavioral analysis: record action and analyze for anomalies
	if h.behavioralRegistry != nil && sessionID != "" {
		sa := h.behavioralRegistry.GetOrCreate(sessionID)
		sa.RecordAction(&envelope.ActionEnvelope{
			Timestamp: time.Now().UTC(),
			Tool:      req.Model,
			Target:    "chat-completion",
			Protocol:  envelope.ProtocolHTTP,
			Actor:     envelope.ActorInfo{TenantID: tenantID, SessionID: sessionID},
		})
		sa.Analyze()
	}

	// Record analytics data point
	h.recordAnalytics(tenantID, req.Model, providerName, 200, startTime, int64(resp.Usage.TotalTokens), qualityScore)
	h.logRequest(startTime, r, tenantID, req.Model, providerName, http.StatusOK, resp.Usage.TotalTokens, false, routed.Region)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-AegisFlow-Cache", "MISS")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request, req *types.ChatCompletionRequest, tenantID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "server_error", "streaming not supported")
		return
	}

	stream, err := h.router.RouteStream(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "provider_error", err.Error())
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Streaming with output policy scanning
	var accumulated strings.Builder
	buf := make([]byte, 4096)
	checkInterval := 500 // check policy every N bytes
	bytesScanned := 0

	for {
		n, err := stream.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			w.Write(chunk)
			flusher.Flush()

			// Accumulate for policy scanning
			if h.policy != nil {
				accumulated.Write(chunk)
				bytesScanned += n

				if bytesScanned >= checkInterval {
					if v, checkErr := h.policy.CheckOutput(accumulated.String()); checkErr != nil {
						log.Printf("policy engine stream check error: %v", checkErr)
					} else if v != nil {
						if v.Action == policy.ActionBlock {
							h.fireWebhook("stream_policy_violation", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
							// Send error event in SSE format to terminate stream
							errPayload, _ := json.Marshal(map[string]string{
								"error":   "policy_violation",
								"message": v.Message,
							})
							w.Write([]byte("data: "))
							w.Write(errPayload)
							w.Write([]byte("\n\n"))
							flusher.Flush()
							log.Printf("stream terminated: %s", policy.FormatViolation(v))
							return
						}
					}
					bytesScanned = 0
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}
}

func (h *Handler) ListModels(w http.ResponseWriter, r *http.Request) {
	models := h.registry.AllModels()
	resp := types.ModelList{
		Object: "list",
		Data:   models,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) recordAnalytics(tenantID, model, providerName string, statusCode int, startTime time.Time, tokens int64, qualityScore ...int) {
	if h.analytics == nil {
		return
	}
	dp := analytics.DataPoint{
		TenantID:   tenantID,
		Model:      model,
		Provider:   providerName,
		StatusCode: statusCode,
		LatencyMs:  time.Since(startTime).Milliseconds(),
		Tokens:     tokens,
		Timestamp:  time.Now(),
	}
	if len(qualityScore) > 0 {
		dp.QualityScore = qualityScore[0]
	}
	h.analytics.Record(dp)
}

func (h *Handler) fireWebhook(eventType, policyName, action, tenantID, model, message string) {
	if h.webhook == nil {
		return
	}
	h.webhook.Send(webhook.Event{
		EventType:  eventType,
		PolicyName: policyName,
		Action:     action,
		TenantID:   tenantID,
		Model:      model,
		Message:    message,
	})
}

func extractContent(messages []types.Message) string {
	var parts []string
	for _, m := range messages {
		parts = append(parts, m.Content)
	}
	return strings.Join(parts, " ")
}

func (h *Handler) logRequest(startTime time.Time, r *http.Request, tenantID, model, providerName string, status int, tokens int, cached bool, region string) {
	if h.requestLog == nil {
		return
	}
	h.requestLog.Add(admin.RequestEntry{
		Timestamp:    time.Now(),
		RequestID:    chimw.GetReqID(r.Context()),
		TenantID:     tenantID,
		Model:        model,
		Provider:     providerName,
		Region:       region,
		DataPlane:    h.dataPlaneName,
		Status:       status,
		LatencyMs:    time.Since(startTime).Milliseconds(),
		Tokens:       tokens,
		Cached:       cached,
		QualityScore: 0,
	})
}

func writeError(w http.ResponseWriter, code int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(types.NewErrorResponse(code, errType, message))
}
