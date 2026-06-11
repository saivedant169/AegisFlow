package gateway

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"context"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/saivedant169/AegisFlow/internal/admin"
	"github.com/saivedant169/AegisFlow/internal/analytics"
	"github.com/saivedant169/AegisFlow/internal/behavioral"
	"github.com/saivedant169/AegisFlow/internal/cache"
	"github.com/saivedant169/AegisFlow/internal/eval"
	"github.com/saivedant169/AegisFlow/internal/policy"
	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/router"
	"github.com/saivedant169/AegisFlow/internal/storage"
	"github.com/saivedant169/AegisFlow/internal/usage"
	"github.com/saivedant169/AegisFlow/internal/webhook"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

type Handler struct {
	registry             *provider.Registry
	router               *router.Router
	policy               *policy.Engine
	usage                *usage.Tracker
	cache                cache.Cache
	webhook              *webhook.Notifier
	store                *storage.PostgresStore
	dbQueue              chan storage.UsageEvent
	dbWG                 sync.WaitGroup
	analytics            *analytics.Collector
	maxBodySize          int64
	recordSpend          func(tenantID, model string, cost float64)
	budgetCheck          func(tenantID, model string) (bool, []string, string)
	evalBuiltin          bool
	evalMinTokens        int
	evalLatencyMul       float64
	evalWebhook          *eval.WebhookEvaluator
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

	// dbBatchSize is the most events a worker coalesces into one multi-row
	// INSERT; dbFlushInterval bounds how long a partial batch waits so low
	// traffic still gets persisted promptly. dbWorkerCount writers drain the
	// shared queue in parallel.
	dbBatchSize     = 128
	dbFlushInterval = 250 * time.Millisecond
	dbWorkerCount   = 2
)

func NewHandler(registry *provider.Registry, rt *router.Router, pe *policy.Engine, ut *usage.Tracker, c cache.Cache, wh *webhook.Notifier, store *storage.PostgresStore, ac *analytics.Collector, maxBodySize int64, recordSpend func(string, string, float64), budgetCheck func(string, string) (bool, []string, string)) *Handler {
	if maxBodySize <= 0 {
		maxBodySize = defaultMaxBodySize
	}
	h := &Handler{registry: registry, router: rt, policy: pe, usage: ut, cache: c, webhook: wh, store: store, analytics: ac, maxBodySize: maxBodySize, recordSpend: recordSpend, budgetCheck: budgetCheck}
	if store != nil {
		h.dbQueue = make(chan storage.UsageEvent, dbQueueSize)
		for i := 0; i < dbWorkerCount; i++ {
			h.dbWG.Add(1)
			go h.dbWorker()
		}
	}
	return h
}

// dbWorker drains the queue, coalescing events into a single multi-row INSERT
// per batch instead of one round-trip per event. A batch flushes when it fills
// or when dbFlushInterval elapses, so a slow trickle still lands promptly. A
// bounded set of these workers shares the queue, capping goroutine growth.
func (h *Handler) dbWorker() {
	defer h.dbWG.Done()

	batch := make([]storage.UsageEvent, 0, dbBatchSize)
	ticker := time.NewTicker(dbFlushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := h.store.RecordEvents(context.Background(), batch); err != nil {
			log.Printf("db worker: failed to record %d events: %v", len(batch), err)
		}
		batch = batch[:0]
	}

	for {
		select {
		case event, ok := <-h.dbQueue:
			if !ok {
				flush() // queue closed — persist the partial batch before exiting
				return
			}
			batch = append(batch, event)
			if len(batch) >= dbBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

// Close shuts down the handler's background workers cleanly, waiting for queued
// events to be flushed.
func (h *Handler) Close() {
	if h.dbQueue != nil {
		close(h.dbQueue)
		h.dbWG.Wait()
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

	rc := h.buildRequestContext(r, surfaceOpenAI, startTime)
	tenantID := rc.tenantID

	// Input governance: kill-switch, aliasing, transforms, budget, input policy.
	// Shared with the Anthropic path so both wires run the identical sequence.
	gov := h.runInputGovernance(rc, &req)
	if gov.Blocked {
		writeBlockOpenAI(w, gov)
		return
	}
	for _, warning := range gov.Warnings {
		w.Header().Add("X-AegisFlow-Budget-Warning", warning)
	}

	if req.Stream {
		h.handleStream(w, rc, &req)
		return
	}

	// Cache lookup (non-streaming only). On a miss, keep the embedding it
	// computed so the store can reuse it. Shared with the Anthropic path.
	cachedResp, cacheStatus, semanticEmbedding, hit := h.lookupCache(rc, &req)
	if hit {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-AegisFlow-Cache", cacheStatus)
		h.logRequest(startTime, r, tenantID, req.Model, cacheSourceName(cacheStatus), http.StatusOK, cachedResp.Usage.TotalTokens, true, "")
		json.NewEncoder(w).Encode(cachedResp)
		return
	}

	routed, err := h.router.RouteWithProvider(r.Context(), &req)
	if err != nil {
		log.Printf("chat completion: provider routing error: %v", err)
		h.recordAnalytics(tenantID, req.Model, "", http.StatusBadGateway, startTime, 0)
		h.logRequest(startTime, r, tenantID, req.Model, "", http.StatusBadGateway, 0, false, "")
		writeError(w, http.StatusBadGateway, "provider_error", "upstream provider error")
		return
	}
	resp := routed.Response
	providerName := routed.Provider

	// Output policy (shared with the Anthropic path).
	if blk := h.runOutputPolicy(rc, &req, resp, providerName, routed.Region); blk != nil {
		writeBlockOpenAI(w, *blk)
		return
	}

	// Post-response governance: transform, cache, usage/spend/db, eval,
	// behavioral, analytics, audit. Shared with the Anthropic path so the tail
	// can't diverge between wires.
	h.postResponseGovernance(rc, &req, resp, providerName, routed.Region, semanticEmbedding)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-AegisFlow-Cache", "MISS")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleStream(w http.ResponseWriter, rc requestContext, req *types.ChatCompletionRequest) {
	r := rc.httpReq
	tenantID := rc.tenantID
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "server_error", "streaming not supported")
		return
	}

	stream, err := h.router.RouteStream(r.Context(), req)
	if err != nil {
		log.Printf("chat completion stream: provider routing error: %v", err)
		writeError(w, http.StatusBadGateway, "provider_error", "upstream provider error")
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// released counts bytes actually delivered to the client; on a clean finish
	// finalize runs the post-stream governance tail with an estimated token
	// count. A blocked or errored stream returns before finalize, so the tail
	// only runs for a successful response (matching the non-streaming path).
	released := 0
	finalize := func() {
		h.postStreamGovernance(rc, req, "", "", estimateTokensFromBytes(released))
	}

	// Output policy is enforced check-before-release: bytes are buffered and
	// scanned before they go to the client, so blocked content never leaves the
	// gateway. (The old code wrote each chunk first and scanned afterwards,
	// which meant the violating bytes had already reached the client by the
	// time we noticed.) With no policy configured there's nothing to scan, so we
	// pass bytes straight through.
	buf := make([]byte, 4096)

	if h.policy == nil {
		for {
			n, err := stream.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				flusher.Flush()
				released += n
			}
			if err != nil {
				break
			}
		}
		finalize()
		return
	}

	var pending []byte // bytes scanned-but-not-yet-released
	const checkInterval = 500

	emitBlock := func(v *policy.Violation) {
		h.fireWebhook("stream_policy_violation", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
		errPayload, _ := json.Marshal(map[string]string{
			"error":   "policy_violation",
			"message": v.Message,
		})
		w.Write([]byte("data: "))
		w.Write(errPayload)
		w.Write([]byte("\n\n"))
		flusher.Flush()
		log.Printf("stream terminated: %s", policy.FormatViolation(v))
	}

	flushPending := func() {
		if len(pending) > 0 {
			released += len(pending)
			w.Write(pending)
			flusher.Flush()
			pending = pending[:0]
		}
	}

	// When every output filter supports incremental matching, scan each chunk
	// once as it arrives (O(total bytes)) instead of re-scanning the whole
	// accumulated window every checkInterval (O(n^2)). Bytes are still scanned
	// before release, so blocked content never egresses.
	if matcher, ok := h.policy.NewOutputStreamMatcher(); ok {
		for {
			n, err := stream.Read(buf)
			if n > 0 {
				if v := matcher.Write(buf[:n]); v != nil && v.Action == policy.ActionBlock {
					emitBlock(v)
					return
				}
				pending = append(pending, buf[:n]...)
				if len(pending) >= checkInterval {
					flushPending()
				}
			}
			if err != nil {
				break
			}
		}
		if v := matcher.Close(); v != nil && v.Action == policy.ActionBlock {
			emitBlock(v)
			return
		}
		flushPending()
		finalize()
		return
	}

	// Fallback: some output filter can match an unbounded span (regex/PII/WASM),
	// so re-scan a bounded accumulated window before releasing each batch.
	var scanWindow strings.Builder // bounded tail used for the policy scan

	// release scans the current window and, unless blocked, sends the buffered
	// bytes to the client. Returns false if the stream was blocked (the caller
	// must stop); the SSE error event is already written in that case.
	release := func() bool {
		if len(pending) == 0 {
			return true
		}
		if v, checkErr := h.policy.CheckOutput(scanWindow.String()); checkErr != nil {
			log.Printf("policy engine stream check error: %v", checkErr)
		} else if v != nil && v.Action == policy.ActionBlock {
			emitBlock(v)
			return false
		}
		flushPending()
		return true
	}

	for {
		n, err := stream.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			scanWindow.Write(buf[:n])
			if scanWindow.Len() > maxAccumulatedStreamBytes {
				s := scanWindow.String()
				scanWindow.Reset()
				scanWindow.WriteString(s[len(s)-maxAccumulatedStreamBytes:])
			}
			if len(pending) >= checkInterval {
				if !release() {
					return
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
	// Scan and release whatever's left below the interval.
	release()
	finalize()
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
