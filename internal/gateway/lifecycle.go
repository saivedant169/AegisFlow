package gateway

import (
	"log"
	"net/http"
	"time"

	"github.com/saivedant169/AegisFlow/internal/cache"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/eval"
	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/storage"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

// apiSurface identifies the wire format a request arrived on. It is used only
// for audit/analytics tagging and never to branch governance — both surfaces
// must run the identical pipeline.
type apiSurface int

const (
	surfaceOpenAI apiSurface = iota
	surfaceAnthropic
)

// behavioralTarget labels the action recorded for behavioral analysis so the
// admin view can tell the wire apart; it does not affect any decision.
func (s apiSurface) behavioralTarget() string {
	if s == surfaceAnthropic {
		return "messages"
	}
	return "chat-completion"
}

// requestContext bundles the cross-cutting inputs every governance stage needs,
// so the shared pipeline doesn't depend on which wire adapter called it. It is a
// plain value (kept on the stack) to avoid per-request heap allocation.
type requestContext struct {
	tenantID  string
	sessionID string
	startTime time.Time
	httpReq   *http.Request
	surface   apiSurface
}

// buildRequestContext resolves the tenant from the request context and derives
// the session id. sessionID aliases tenantID so an Anthropic request advances
// the same behavioral analyzer an OpenAI request created (single source of
// truth for the aliasing that used to live inline in ChatCompletion).
func (h *Handler) buildRequestContext(r *http.Request, surface apiSurface, startTime time.Time) requestContext {
	tenantID := ""
	if t := middleware.TenantFromContext(r.Context()); t != nil {
		tenantID = t.ID
	}
	return requestContext{
		tenantID:  tenantID,
		sessionID: tenantID,
		startTime: startTime,
		httpReq:   r,
		surface:   surface,
	}
}

// postResponseGovernance runs every side effect that must follow a successful,
// policy-passed response: response transform, cache population, usage/spend/db
// accounting, quality eval, behavioral recording, and analytics/audit logging.
//
// It writes no HTTP response — the wire adapter serializes resp afterward. Both
// the OpenAI and Anthropic paths call this so the post-response governance tail
// can't drift between wires (previously only the OpenAI path ran it, so
// /v1/messages traffic skipped spend, db, eval, behavioral, and cache entirely).
// The order mirrors the original ChatCompletion tail exactly.
func (h *Handler) postResponseGovernance(rc requestContext, req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse, providerName, region string, semanticEmbedding []float64) {
	// Apply response transformations.
	if h.responseTransformCfg != nil {
		TransformResponse(resp, h.responseTransformCfg)
	}

	// Cache the response (exact + semantic).
	if h.cache != nil {
		cacheKey := cache.BuildKey(rc.tenantID, req.Model, req.Messages)
		h.cache.Set(cacheKey, resp)
	}
	if h.semanticCache != nil {
		h.semanticCache.StoreAsync(rc.tenantID, req, resp, semanticEmbedding)
	}

	// Track in-memory usage.
	if h.usage != nil {
		h.usage.Record(rc.tenantID, providerName, req.Model, resp.Usage)
	}

	// Persist to database via the buffered worker queue (non-blocking).
	if h.dbQueue != nil {
		select {
		case h.dbQueue <- storage.UsageEvent{
			TenantID: rc.tenantID, Model: req.Model,
			PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens: resp.Usage.TotalTokens, StatusCode: 200,
			LatencyMs: time.Since(rc.startTime).Milliseconds(), CreatedAt: time.Now(),
		}:
		default:
			log.Printf("db queue full — dropping usage event for tenant %s", rc.tenantID)
		}
	}

	// Record spend for budget tracking.
	if h.recordSpend != nil {
		h.recordSpend(rc.tenantID, req.Model, float64(resp.Usage.TotalTokens)*0.00001)
	}

	// Quality evaluation.
	qualityScore := 0
	if h.evalBuiltin {
		evalResult := eval.ScoreResponse(resp, time.Since(rc.startTime).Milliseconds(), 0, h.evalMinTokens, h.evalLatencyMul)
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
			LatencyMs:    time.Since(rc.startTime).Milliseconds(),
			BuiltinScore: qualityScore,
		})
	}

	// Behavioral analysis: record action and analyze for anomalies.
	if h.behavioralRegistry != nil && rc.sessionID != "" {
		sa := h.behavioralRegistry.GetOrCreate(rc.sessionID)
		sa.RecordAction(&envelope.ActionEnvelope{
			Timestamp: time.Now().UTC(),
			Tool:      req.Model,
			Target:    rc.surface.behavioralTarget(),
			Protocol:  envelope.ProtocolHTTP,
			Actor:     envelope.ActorInfo{TenantID: rc.tenantID, SessionID: rc.sessionID},
		})
		sa.Analyze()
	}

	// Record analytics + admin request-feed log.
	h.recordAnalytics(rc.tenantID, req.Model, providerName, 200, rc.startTime, int64(resp.Usage.TotalTokens), qualityScore)
	h.logRequest(rc.startTime, rc.httpReq, rc.tenantID, req.Model, providerName, http.StatusOK, resp.Usage.TotalTokens, false, region)
}
