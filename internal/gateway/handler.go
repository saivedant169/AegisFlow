package gateway

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/aegisflow/aegisflow/internal/middleware"
	"github.com/aegisflow/aegisflow/internal/policy"
	"github.com/aegisflow/aegisflow/internal/provider"
	"github.com/aegisflow/aegisflow/internal/router"
	"github.com/aegisflow/aegisflow/internal/usage"
	"github.com/aegisflow/aegisflow/pkg/types"
)

type Handler struct {
	registry *provider.Registry
	router   *router.Router
	policy   *policy.Engine
	usage    *usage.Tracker
}

func NewHandler(registry *provider.Registry, rt *router.Router, pe *policy.Engine, ut *usage.Tracker) *Handler {
	return &Handler{registry: registry, router: rt, policy: pe, usage: ut}
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

	// Policy check: input
	if h.policy != nil {
		inputContent := extractContent(req.Messages)
		if v, _ := h.policy.CheckInput(inputContent); v != nil {
			if v.Action == policy.ActionBlock {
				writeError(w, http.StatusForbidden, "policy_violation", policy.FormatViolation(v))
				return
			}
			log.Printf("policy warning: %s", policy.FormatViolation(v))
		}
	}

	if req.Stream {
		h.handleStream(w, r, &req)
		return
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
				writeError(w, http.StatusForbidden, "policy_violation", policy.FormatViolation(v))
				return
			}
			log.Printf("policy warning (output): %s", policy.FormatViolation(v))
		}
	}

	// Track usage
	if h.usage != nil {
		tenantID := ""
		if t := middleware.TenantFromContext(r.Context()); t != nil {
			tenantID = t.ID
		}
		h.usage.Record(tenantID, req.Model, resp.Usage)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleStream(w http.ResponseWriter, r *http.Request, req *types.ChatCompletionRequest) {
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

	buf := make([]byte, 4096)
	for {
		n, err := stream.Read(buf)
		if n > 0 {
			w.Write(buf[:n])
			flusher.Flush()
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
