package gateway

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/aegisflow/aegisflow/internal/cache"
	"github.com/aegisflow/aegisflow/internal/middleware"
	"github.com/aegisflow/aegisflow/internal/policy"
	"github.com/aegisflow/aegisflow/internal/provider"
	"github.com/aegisflow/aegisflow/internal/router"
	"github.com/aegisflow/aegisflow/internal/usage"
	"github.com/aegisflow/aegisflow/internal/webhook"
	"github.com/aegisflow/aegisflow/pkg/types"
)

type Handler struct {
	registry *provider.Registry
	router   *router.Router
	policy   *policy.Engine
	usage    *usage.Tracker
	cache    cache.Cache
	webhook  *webhook.Notifier
}

func NewHandler(registry *provider.Registry, rt *router.Router, pe *policy.Engine, ut *usage.Tracker, c cache.Cache, wh *webhook.Notifier) *Handler {
	return &Handler{registry: registry, router: rt, policy: pe, usage: ut, cache: c, webhook: wh}
}

func (h *Handler) ChatCompletion(w http.ResponseWriter, r *http.Request) {
	var req types.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
		cacheKey := cache.BuildKey(req.Model, req.Messages)
		if cached, ok := h.cache.Get(cacheKey); ok {
			log.Printf("cache hit: %s", cacheKey[:20])
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-AegisFlow-Cache", "HIT")
			json.NewEncoder(w).Encode(cached)
			return
		}
	}

	resp, err := h.router.Route(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "provider_error", err.Error())
		return
	}

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
		cacheKey := cache.BuildKey(req.Model, req.Messages)
		h.cache.Set(cacheKey, resp)
	}

	// Track usage
	if h.usage != nil {
		h.usage.Record(tenantID, req.Model, resp.Usage)
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
							errChunk := `data: {"error":"policy_violation","message":"` + v.Message + `"}` + "\n\n"
							w.Write([]byte(errChunk))
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
