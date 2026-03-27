package gateway

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"context"
	"time"

	"github.com/aegisflow/aegisflow/internal/analytics"
	"github.com/aegisflow/aegisflow/internal/cache"
	"github.com/aegisflow/aegisflow/internal/middleware"
	"github.com/aegisflow/aegisflow/internal/policy"
	"github.com/aegisflow/aegisflow/internal/provider"
	"github.com/aegisflow/aegisflow/internal/router"
	"github.com/aegisflow/aegisflow/internal/storage"
	"github.com/aegisflow/aegisflow/internal/usage"
	"github.com/aegisflow/aegisflow/internal/webhook"
	"github.com/aegisflow/aegisflow/pkg/types"
)

type Handler struct {
	registry  *provider.Registry
	router    *router.Router
	policy    *policy.Engine
	usage     *usage.Tracker
	cache     cache.Cache
	webhook   *webhook.Notifier
	store     *storage.PostgresStore
	dbQueue   chan storage.UsageEvent
	analytics *analytics.Collector
}

const dbQueueSize = 1024

func NewHandler(registry *provider.Registry, rt *router.Router, pe *policy.Engine, ut *usage.Tracker, c cache.Cache, wh *webhook.Notifier, store *storage.PostgresStore, ac *analytics.Collector) *Handler {
	h := &Handler{registry: registry, router: rt, policy: pe, usage: ut, cache: c, webhook: wh, store: store, analytics: ac}
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

func (h *Handler) ChatCompletion(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	const maxBodySize = 10 * 1024 * 1024 // 10MB
	var req types.ChatCompletionRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodySize)).Decode(&req); err != nil {
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

	// Policy check: input
	if h.policy != nil {
		inputContent := extractContent(req.Messages)
		if v, _ := h.policy.CheckInput(inputContent); v != nil {
			if v.Action == policy.ActionBlock {
				h.fireWebhook("policy_violation", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
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

	// Check cache (non-streaming only)
	if h.cache != nil {
		cacheKey := cache.BuildKey(tenantID, req.Model, req.Messages)
		if cached, ok := h.cache.Get(cacheKey); ok {
			log.Printf("cache hit: %s", cacheKey[:20])
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-AegisFlow-Cache", "HIT")
			json.NewEncoder(w).Encode(cached)
			return
		}
	}

	routed, err := h.router.RouteWithProvider(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "provider_error", err.Error())
		return
	}
	resp := routed.Response
	providerName := routed.Provider

	// Policy check: output
	if h.policy != nil && len(resp.Choices) > 0 {
		if v, _ := h.policy.CheckOutput(resp.Choices[0].Message.Content); v != nil {
			if v.Action == policy.ActionBlock {
				h.fireWebhook("policy_violation", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
				writeError(w, http.StatusForbidden, "policy_violation", policy.FormatViolation(v))
				return
			}
			log.Printf("policy warning (output): %s", policy.FormatViolation(v))
		}
	}

	// Cache the response
	if h.cache != nil {
		cacheKey := cache.BuildKey(tenantID, req.Model, req.Messages)
		h.cache.Set(cacheKey, resp)
	}

	// Track usage
	if h.usage != nil {
		h.usage.Record(tenantID, req.Model, resp.Usage)
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

	// Record analytics data point
	if h.analytics != nil {
		h.analytics.Record(analytics.DataPoint{
			TenantID:      tenantID,
			Model:         req.Model,
			Provider:      providerName,
			StatusCode:    200,
			LatencyMs:     time.Since(startTime).Milliseconds(),
			Tokens:        int64(resp.Usage.TotalTokens),
			EstimatedCost: 0,
			Timestamp:     time.Now(),
		})
	}

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
					if v, _ := h.policy.CheckOutput(accumulated.String()); v != nil {
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

func writeError(w http.ResponseWriter, code int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(types.NewErrorResponse(code, errType, message))
}
