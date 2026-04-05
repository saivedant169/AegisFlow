package gateway

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"golang.org/x/net/websocket"

	"github.com/saivedant169/AegisFlow/internal/cache"
	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/eval"
	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/policy"
	"github.com/saivedant169/AegisFlow/internal/storage"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

// WebSocketConfig holds configuration for the WebSocket endpoint.
type WebSocketConfig struct {
	Enabled      bool          `yaml:"enabled"`
	PingInterval time.Duration `yaml:"ping_interval"`
}

// wsEnvelope wraps messages sent over the WebSocket connection.
// Incoming messages have type "request"; outgoing messages have type
// "response", "error", or "ping"/"pong".
type wsEnvelope struct {
	Type    string           `json:"type"`
	ID      string           `json:"id,omitempty"`
	Payload *json.RawMessage `json:"payload,omitempty"`
}

// WebSocket returns an http.Handler that upgrades to WebSocket and processes
// chat-completion requests using the same pipeline as the HTTP handler.
// Auth is resolved from query param "api_key" or from the first message if
// the query param is absent.
func (h *Handler) WebSocket(cfg *config.Config, wsCfg WebSocketConfig) http.Handler {
	pingInterval := wsCfg.PingInterval
	if pingInterval <= 0 {
		pingInterval = 30 * time.Second
	}

	server := websocket.Server{
		Handshake: func(ws *websocket.Config, r *http.Request) error {
			// Accept any origin (CORS is handled at the HTTP layer).
			ws.Origin, _ = websocket.Origin(ws, r)
			return nil
		},
		Handler: func(conn *websocket.Conn) {
			h.handleWebSocket(conn, cfg, pingInterval)
		},
	}
	return server
}

func (h *Handler) handleWebSocket(conn *websocket.Conn, cfg *config.Config, pingInterval time.Duration) {
	defer conn.Close()

	// Attempt auth from query param.
	var tenant *config.TenantConfig
	apiKey := conn.Request().URL.Query().Get("api_key")
	if apiKey != "" {
		if match := cfg.FindTenantByAPIKey(apiKey); match != nil {
			tenant = match.Tenant
		} else {
			h.wsSendError(conn, "", http.StatusUnauthorized, "authentication_error", "invalid API key")
			return
		}
	}

	// Ping ticker for keepalive.
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	// Channel to receive decoded messages from the read goroutine.
	type readResult struct {
		env wsEnvelope
		err error
	}
	readCh := make(chan readResult, 1)

	go func() {
		for {
			var raw []byte
			err := websocket.Message.Receive(conn, &raw)
			if err != nil {
				readCh <- readResult{err: err}
				return
			}
			var env wsEnvelope
			if err := json.Unmarshal(raw, &env); err != nil {
				readCh <- readResult{env: wsEnvelope{Type: "__invalid__"}}
				continue
			}
			readCh <- readResult{env: env}
		}
	}()

	// If no api_key query param, the first message must be an auth message.
	if tenant == nil {
		select {
		case res := <-readCh:
			if res.err != nil {
				return
			}
			if res.env.Type != "auth" || res.env.Payload == nil {
				h.wsSendError(conn, res.env.ID, http.StatusUnauthorized, "authentication_error", "first message must be type \"auth\" with api_key in payload, or pass api_key query param")
				return
			}
			var authMsg struct {
				APIKey string `json:"api_key"`
			}
			if err := json.Unmarshal(*res.env.Payload, &authMsg); err != nil || authMsg.APIKey == "" {
				h.wsSendError(conn, res.env.ID, http.StatusUnauthorized, "authentication_error", "auth payload must contain api_key")
				return
			}
			match := cfg.FindTenantByAPIKey(authMsg.APIKey)
			if match == nil {
				h.wsSendError(conn, res.env.ID, http.StatusUnauthorized, "authentication_error", "invalid API key")
				return
			}
			tenant = match.Tenant
			// Send auth success.
			h.wsSend(conn, wsEnvelope{Type: "auth_ok", ID: res.env.ID})
		case <-time.After(10 * time.Second):
			h.wsSendError(conn, "", http.StatusUnauthorized, "authentication_error", "auth timeout")
			return
		}
	}

	// Build a base context with tenant info.
	baseCtx := context.WithValue(context.Background(), middleware.TenantContextKey, tenant)

	for {
		select {
		case <-pingTicker.C:
			h.wsSend(conn, wsEnvelope{Type: "ping"})

		case res := <-readCh:
			if res.err != nil {
				// Connection closed or read error.
				return
			}

			switch res.env.Type {
			case "pong":
				// Client responded to our ping; nothing to do.
				continue
			case "ping":
				h.wsSend(conn, wsEnvelope{Type: "pong", ID: res.env.ID})
				continue
			case "__invalid__":
				h.wsSendError(conn, "", http.StatusBadRequest, "invalid_request", "failed to parse message envelope")
				continue
			case "request":
				// Process chat completion request.
			default:
				h.wsSendError(conn, res.env.ID, http.StatusBadRequest, "invalid_request", "unknown message type: "+res.env.Type)
				continue
			}

			if res.env.Payload == nil {
				h.wsSendError(conn, res.env.ID, http.StatusBadRequest, "invalid_request", "missing payload")
				continue
			}

			var req types.ChatCompletionRequest
			if err := json.Unmarshal(*res.env.Payload, &req); err != nil {
				h.wsSendError(conn, res.env.ID, http.StatusBadRequest, "invalid_request", "failed to parse request payload")
				continue
			}

			resp, errResp := h.processWSRequest(baseCtx, tenant.ID, &req)
			if errResp != nil {
				h.wsSend(conn, *errResp)
				continue
			}

			payloadBytes, _ := json.Marshal(resp)
			raw := json.RawMessage(payloadBytes)
			h.wsSend(conn, wsEnvelope{
				Type:    "response",
				ID:      res.env.ID,
				Payload: &raw,
			})
		}
	}
}

// processWSRequest runs the same pipeline as ChatCompletion but returns data
// instead of writing to an http.ResponseWriter. Returns either a response or
// an error envelope.
func (h *Handler) processWSRequest(ctx context.Context, tenantID string, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, *wsEnvelope) {
	startTime := time.Now()

	if req.Model == "" {
		return nil, h.wsErrorEnvelope("", http.StatusBadRequest, "invalid_request", "model is required")
	}
	if len(req.Messages) == 0 {
		return nil, h.wsErrorEnvelope("", http.StatusBadRequest, "invalid_request", "messages is required")
	}

	// Force non-streaming for WebSocket (responses go as complete JSON).
	req.Stream = false

	// Model aliasing
	if h.modelAliases != nil {
		ApplyModelAlias(req, h.modelAliases)
	}

	// Request transforms
	var tenantTransform *TransformConfig
	if tenantID != "" && h.tenantTransforms != nil {
		tenantTransform = h.tenantTransforms[tenantID]
	}
	TransformRequestWithTenant(req, h.transformCfg, tenantTransform)

	// Budget check
	if h.budgetCheck != nil && tenantID != "" {
		allowed, _, blockMsg := h.budgetCheck(tenantID, req.Model)
		if !allowed {
			h.recordAnalytics(tenantID, req.Model, "", http.StatusTooManyRequests, startTime, 0)
			return nil, h.wsErrorEnvelope("", http.StatusTooManyRequests, "budget_exceeded", blockMsg)
		}
	}

	// Policy check: input
	if h.policy != nil {
		inputContent := extractContent(req.Messages)
		if v, _ := h.policy.CheckInput(inputContent); v != nil {
			if v.Action == policy.ActionBlock {
				h.fireWebhook("policy_violation", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
				h.recordAnalytics(tenantID, req.Model, "", http.StatusForbidden, startTime, 0)
				return nil, h.wsErrorEnvelope("", http.StatusForbidden, "policy_violation", policy.FormatViolation(v))
			}
			h.fireWebhook("policy_warning", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
			log.Printf("policy warning: %s", policy.FormatViolation(v))
		}
	}

	// Route to provider
	routed, err := h.router.RouteWithProvider(ctx, req)
	if err != nil {
		h.recordAnalytics(tenantID, req.Model, "", http.StatusBadGateway, startTime, 0)
		return nil, h.wsErrorEnvelope("", http.StatusBadGateway, "provider_error", err.Error())
	}
	resp := routed.Response
	providerName := routed.Provider

	// Policy check: output
	if h.policy != nil && len(resp.Choices) > 0 {
		if v, _ := h.policy.CheckOutput(resp.Choices[0].Message.Content); v != nil {
			if v.Action == policy.ActionBlock {
				h.fireWebhook("policy_violation", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
				h.recordAnalytics(tenantID, req.Model, providerName, http.StatusForbidden, startTime, 0)
				return nil, h.wsErrorEnvelope("", http.StatusForbidden, "policy_violation", policy.FormatViolation(v))
			}
		}
	}

	// Response transforms
	if h.responseTransformCfg != nil {
		TransformResponse(resp, h.responseTransformCfg)
	}

	// Cache
	if h.cache != nil {
		cacheKey := cache.BuildKey(tenantID, req.Model, req.Messages)
		h.cache.Set(cacheKey, resp)
	}

	// Usage tracking
	if h.usage != nil {
		h.usage.Record(tenantID, providerName, req.Model, resp.Usage)
	}

	// DB persistence
	if h.dbQueue != nil {
		select {
		case h.dbQueue <- storage.UsageEvent{
			TenantID: tenantID, Model: req.Model,
			PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens: resp.Usage.TotalTokens, StatusCode: 200,
			LatencyMs: time.Since(startTime).Milliseconds(), CreatedAt: time.Now(),
		}:
		default:
			log.Printf("db queue full - dropping usage event for tenant %s", tenantID)
		}
	}

	// Spend
	if h.recordSpend != nil {
		h.recordSpend(tenantID, req.Model, float64(resp.Usage.TotalTokens)*0.00001)
	}

	// Eval
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

	h.recordAnalytics(tenantID, req.Model, providerName, 200, startTime, int64(resp.Usage.TotalTokens), qualityScore)

	return resp, nil
}

func (h *Handler) wsSend(conn *websocket.Conn, env wsEnvelope) {
	data, err := json.Marshal(env)
	if err != nil {
		return
	}
	websocket.Message.Send(conn, string(data))
}

func (h *Handler) wsSendError(conn *websocket.Conn, id string, code int, errType, message string) {
	env := h.wsErrorEnvelopeWithID(id, code, errType, message)
	h.wsSend(conn, *env)
}

func (h *Handler) wsErrorEnvelope(id string, code int, errType, message string) *wsEnvelope {
	return h.wsErrorEnvelopeWithID(id, code, errType, message)
}

func (h *Handler) wsErrorEnvelopeWithID(id string, code int, errType, message string) *wsEnvelope {
	errResp := types.NewErrorResponse(code, errType, message)
	data, _ := json.Marshal(errResp)
	raw := json.RawMessage(data)
	return &wsEnvelope{
		Type:    "error",
		ID:      id,
		Payload: &raw,
	}
}
