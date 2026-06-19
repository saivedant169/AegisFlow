package gateway

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/saivedant169/AegisFlow/internal/cache"
	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/eval"
	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/policy"
	"github.com/saivedant169/AegisFlow/internal/storage"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

// governableInput gathers every text surface in a request that input policy must
// inspect: message contents, tool-call arguments carried on assistant messages,
// and the tool definitions themselves. A blocked keyword or injection hidden in
// a tool name/description/schema or in tool-call arguments would otherwise slip
// past CheckInput, which historically saw only Message.Content.
func governableInput(req *types.ChatCompletionRequest) string {
	var b strings.Builder
	for _, m := range req.Messages {
		if m.Content != "" {
			b.WriteString(m.Content)
			b.WriteByte('\n')
		}
		for _, tc := range m.ToolCalls {
			b.WriteString(tc.Function.Name)
			b.WriteByte(' ')
			writeScannable(&b, []byte(tc.Function.Arguments))
			b.WriteByte('\n')
		}
	}
	for _, t := range req.Tools {
		b.WriteString(t.Function.Name)
		b.WriteByte(' ')
		b.WriteString(t.Function.Description)
		b.WriteByte(' ')
		writeScannable(&b, t.Function.Parameters)
		b.WriteByte('\n')
	}
	if len(req.ToolChoice) > 0 && string(req.ToolChoice) != "null" {
		writeScannable(&b, req.ToolChoice)
		b.WriteByte('\n')
	}
	return b.String()
}

// governableOutput gathers a response message's text plus any tool call the
// model requested, so output policy scans tool-call arguments — an exfiltration
// channel — not just the assistant's text content.
func governableOutput(msg *types.Message) string {
	var b strings.Builder
	b.WriteString(msg.Content)
	for _, tc := range msg.ToolCalls {
		b.WriteByte('\n')
		b.WriteString(tc.Function.Name)
		b.WriteByte(' ')
		writeScannable(&b, []byte(tc.Function.Arguments))
	}
	return b.String()
}

// writeScannable appends raw JSON bytes AND their decoded form to b. Scanning
// the raw bytes preserves coverage of structural text; scanning the decoded form
// resolves JSON unicode escapes to their literal characters, so a keyword hidden
// behind \u escapes — which the provider's parser would decode — can't slip past
// the policy filters.
func writeScannable(b *strings.Builder, raw []byte) {
	if len(raw) == 0 {
		return
	}
	b.Write(raw)
	if decoded := decodeJSONForScan(raw); decoded != "" {
		b.WriteByte(' ')
		b.WriteString(decoded)
	}
}

// decodeJSONForScan returns the canonical re-encoding of valid JSON (escapes
// resolved to literal characters), or "" if raw isn't valid JSON.
func decodeJSONForScan(raw []byte) string {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return ""
	}
	out, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(out)
}

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

// auditAPI tags the audit-log entry with the wire the request arrived on. Empty
// for OpenAI (the original ChatCompletion path logged no api tag).
func (s apiSurface) auditAPI() string {
	if s == surfaceAnthropic {
		return "messages"
	}
	return ""
}

// govBlockKind is the semantic reason input governance rejected a request. Each
// wire adapter maps it to its own error envelope, so the decision is made once
// and rendered per-wire.
type govBlockKind int

const (
	blockNone govBlockKind = iota
	blockKillSwitch
	blockBudget
	blockInputPolicy
	blockPolicyError
)

// govResult is the outcome of input governance. Blocked means the adapter must
// stop and render an error built from Kind/Status/Message; otherwise Warnings
// (budget warnings) should be surfaced as response headers. The wire-agnostic
// side effects (webhook, audit, analytics-on-block) already fired inside
// runInputGovernance.
type govResult struct {
	Blocked  bool
	Status   int
	Kind     govBlockKind
	Message  string
	Warnings []string
}

// runInputGovernance runs the pre-route preparation and gates that must apply to
// every request regardless of wire: behavioral kill-switch, model aliasing,
// request transforms, per-model budget, and input policy. It mutates req in
// place (aliasing/transforms) and returns the decision. Order matches the
// original ChatCompletion sequence exactly. Shared so /v1/messages can't drift
// from /v1/chat/completions (it previously skipped kill-switch and budget).
func (h *Handler) runInputGovernance(rc requestContext, req *types.ChatCompletionRequest) govResult {
	// logBlock records a blocked request to the admin feed so operators see
	// input-stage blocks, not just output-stage ones (and not just successes).
	logBlock := func(status int) {
		h.logRequest(rc.startTime, rc.httpReq, rc.tenantID, req.Model, "", status, 0, false, "")
	}

	// Behavioral kill-switch.
	if h.behavioralRegistry != nil && rc.sessionID != "" {
		sa := h.behavioralRegistry.GetOrCreate(rc.sessionID)
		if sa.Blocked() {
			logBlock(http.StatusForbidden)
			return govResult{
				Blocked: true, Status: http.StatusForbidden, Kind: blockKillSwitch,
				Message: "session blocked by behavioral kill switch — cumulative risk score exceeded threshold",
			}
		}
	}

	// Model aliasing.
	if h.modelAliases != nil {
		ApplyModelAlias(req, h.modelAliases)
	}

	// Request transformations (per-tenant overrides global).
	var tenantTransform *TransformConfig
	if rc.tenantID != "" && h.tenantTransforms != nil {
		tenantTransform = h.tenantTransforms[rc.tenantID]
	}
	TransformRequestWithTenant(req, h.transformCfg, tenantTransform)

	// Per-model budget check.
	var warnings []string
	if h.budgetCheck != nil && rc.tenantID != "" {
		allowed, w, blockMsg := h.budgetCheck(rc.tenantID, req.Model)
		if !allowed {
			h.recordAnalytics(rc.tenantID, req.Model, "", http.StatusTooManyRequests, rc.startTime, 0)
			logBlock(http.StatusTooManyRequests)
			return govResult{Blocked: true, Status: http.StatusTooManyRequests, Kind: blockBudget, Message: blockMsg}
		}
		warnings = w
	}

	// Input policy.
	if h.policy != nil {
		inputContent := governableInput(req)
		v, err := h.policy.CheckInput(inputContent)
		if err != nil {
			log.Printf("policy engine input check error: %v", err)
			h.recordAnalytics(rc.tenantID, req.Model, "", http.StatusInternalServerError, rc.startTime, 0)
			logBlock(http.StatusInternalServerError)
			return govResult{Blocked: true, Status: http.StatusInternalServerError, Kind: blockPolicyError, Message: "policy engine error"}
		}
		if v != nil {
			if v.Action == policy.ActionBlock {
				h.fireWebhook("policy_violation", v.PolicyName, string(v.Action), rc.tenantID, req.Model, v.Message)
				h.recordAnalytics(rc.tenantID, req.Model, "", http.StatusForbidden, rc.startTime, 0)
				if h.auditLog != nil {
					detail := map[string]string{"message": v.Message, "prompt": runeTruncate(inputContent, 200)}
					if api := rc.surface.auditAPI(); api != "" {
						detail["api"] = api
					}
					h.auditLog("system", "system", "policy.block", "policy:"+v.PolicyName, auditDetail(detail), rc.tenantID, req.Model)
				}
				logBlock(http.StatusForbidden)
				return govResult{Blocked: true, Status: http.StatusForbidden, Kind: blockInputPolicy, Message: policy.FormatViolation(v)}
			}
			h.fireWebhook("policy_warning", v.PolicyName, string(v.Action), rc.tenantID, req.Model, v.Message)
			log.Printf("policy warning: %s", policy.FormatViolation(v))
		}
	}

	return govResult{Warnings: warnings}
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

// runOutputPolicy runs the output policy on a completed response. It returns a
// non-nil blocked govResult if the response must be rejected, having already
// fired the wire-agnostic side effects (webhook, analytics, audit, request log).
// nil means the response may be served. Shared so /v1/messages logs an
// output-block to the admin feed exactly like /v1/chat/completions.
func (h *Handler) runOutputPolicy(rc requestContext, req *types.ChatCompletionRequest, resp *types.ChatCompletionResponse, providerName, region string) *govResult {
	if h.policy == nil || len(resp.Choices) == 0 {
		return nil
	}
	v, err := h.policy.CheckOutput(governableOutput(&resp.Choices[0].Message))
	if err != nil {
		log.Printf("policy engine output check error: %v", err)
		h.recordAnalytics(rc.tenantID, req.Model, providerName, http.StatusInternalServerError, rc.startTime, 0)
		return &govResult{Blocked: true, Status: http.StatusInternalServerError, Kind: blockPolicyError, Message: "policy engine error"}
	}
	if v != nil && v.Action == policy.ActionBlock {
		h.fireWebhook("policy_violation", v.PolicyName, string(v.Action), rc.tenantID, req.Model, v.Message)
		h.recordAnalytics(rc.tenantID, req.Model, providerName, http.StatusForbidden, rc.startTime, 0)
		h.logRequest(rc.startTime, rc.httpReq, rc.tenantID, req.Model, providerName, http.StatusForbidden, resp.Usage.TotalTokens, false, region)
		if h.auditLog != nil {
			detail := map[string]string{"message": v.Message, "phase": "output"}
			if api := rc.surface.auditAPI(); api != "" {
				detail["api"] = api
			}
			h.auditLog("system", "system", "policy.block", "policy:"+v.PolicyName, auditDetail(detail), rc.tenantID, req.Model)
		}
		return &govResult{Blocked: true, Status: http.StatusForbidden, Kind: blockInputPolicy, Message: policy.FormatViolation(v)}
	}
	if v != nil {
		log.Printf("policy warning (output): %s", policy.FormatViolation(v))
	}
	return nil
}

// lookupCache checks the semantic cache then the exact cache. On a hit it
// returns the cached response and a wire-agnostic status ("SEMANTIC-HIT" /
// "HIT"); on a miss it returns the embedding computed during the semantic
// lookup so the store can reuse it instead of embedding the same text again.
// Shared so /v1/messages reads the same cache /v1/chat/completions warms.
func (h *Handler) lookupCache(rc requestContext, req *types.ChatCompletionRequest) (resp *types.ChatCompletionResponse, status string, embedding []float64, hit bool) {
	if h.semanticCache != nil {
		cached, emb, ok := h.semanticCache.GetSemanticWithEmbedding(rc.tenantID, req)
		if ok {
			return cached, "SEMANTIC-HIT", nil, true
		}
		embedding = emb
	}
	if h.cache != nil {
		key := cache.BuildKey(rc.tenantID, req.Model, req.Messages)
		if cached, ok := h.cache.Get(key); ok {
			return cached, "HIT", nil, true
		}
	}
	return nil, "", embedding, false
}

// cacheSourceName maps a cache status to the provider label used in the request
// log, preserving the original ChatCompletion labels.
func cacheSourceName(status string) string {
	if status == "SEMANTIC-HIT" {
		return "semantic-cache"
	}
	return "cache"
}

// writeBlockOpenAI renders a govResult block as an OpenAI-wire error,
// preserving the original ChatCompletion error-type strings per block kind.
func writeBlockOpenAI(w http.ResponseWriter, gov govResult) {
	switch gov.Kind {
	case blockKillSwitch:
		writeError(w, gov.Status, "session_blocked", gov.Message)
	case blockBudget:
		writeError(w, gov.Status, "budget_exceeded", gov.Message)
	case blockInputPolicy:
		writeError(w, gov.Status, "policy_violation", gov.Message)
	default: // blockPolicyError
		writeError(w, gov.Status, "policy_error", gov.Message)
	}
}

// writeBlockAnthropic renders a govResult block as an Anthropic-wire error.
// Input-policy and policy-engine-error map to the same types the original
// Messages handler used; kill-switch and budget are new on this wire.
func writeBlockAnthropic(w http.ResponseWriter, gov govResult) {
	switch gov.Kind {
	case blockKillSwitch:
		writeAnthropicError(w, gov.Status, "permission_error", gov.Message)
	case blockBudget:
		writeAnthropicError(w, gov.Status, "rate_limit_error", gov.Message)
	case blockInputPolicy:
		writeAnthropicError(w, gov.Status, "permission_error", gov.Message)
	default: // blockPolicyError
		writeAnthropicError(w, gov.Status, "api_error", gov.Message)
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

// postStreamGovernance runs the governance tail for a cleanly completed stream:
// usage, db persistence, spend, behavioral record/analyze, analytics, and the
// request log. It omits the stages that don't apply to streamed output — cache
// population (streams aren't cached), response transform, and quality eval
// (there is no buffered body to score). outTokens is the estimated output token
// count. Shared by both streaming wires, which previously ran none of this, so
// streamed traffic was invisible to spend, db, and behavioral defense.
func (h *Handler) postStreamGovernance(rc requestContext, req *types.ChatCompletionRequest, providerName, region string, outTokens int) {
	usageData := types.Usage{CompletionTokens: outTokens, TotalTokens: outTokens}

	if h.usage != nil {
		h.usage.Record(rc.tenantID, providerName, req.Model, usageData)
	}
	if h.dbQueue != nil {
		select {
		case h.dbQueue <- storage.UsageEvent{
			TenantID: rc.tenantID, Model: req.Model,
			CompletionTokens: outTokens, TotalTokens: outTokens, StatusCode: 200,
			LatencyMs: time.Since(rc.startTime).Milliseconds(), CreatedAt: time.Now(),
		}:
		default:
			log.Printf("db queue full — dropping usage event for tenant %s", rc.tenantID)
		}
	}
	if h.recordSpend != nil {
		h.recordSpend(rc.tenantID, req.Model, float64(outTokens)*0.00001)
	}
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
	h.recordAnalytics(rc.tenantID, req.Model, providerName, 200, rc.startTime, int64(outTokens))
	h.logRequest(rc.startTime, rc.httpReq, rc.tenantID, req.Model, providerName, http.StatusOK, outTokens, false, region)
}
