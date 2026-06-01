package gateway

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/saivedant169/AegisFlow/internal/middleware"
	"github.com/saivedant169/AegisFlow/internal/policy"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

// This file adds an inbound Anthropic Messages API endpoint (POST /v1/messages)
// so Anthropic-native clients — Claude Code (via ANTHROPIC_BASE_URL) and the
// Anthropic SDK — can route through AegisFlow and have their prompts governed
// by the same policy + audit pipeline the OpenAI-compatible endpoint uses.
//
// The handler translates the Anthropic request into the internal
// ChatCompletionRequest, runs input policy + audit, routes to a provider,
// runs output policy, then translates the result back into Anthropic
// Messages format. It deliberately reuses the existing components
// (h.policy, h.router, h.usage, h.auditLog) and does not touch the proven
// OpenAI handler.

// --- Inbound request types (Anthropic Messages API) ---

type anthropicMessagesRequest struct {
	Model         string               `json:"model"`
	MaxTokens     int                  `json:"max_tokens"`
	Messages      []anthropicInMessage `json:"messages"`
	System        json.RawMessage      `json:"system,omitempty"`
	Stream        bool                 `json:"stream,omitempty"`
	Temperature   *float64             `json:"temperature,omitempty"`
	TopP          *float64             `json:"top_p,omitempty"`
	StopSequences []string             `json:"stop_sequences,omitempty"`
	// Tool fields are captured only to detect agentic requests we cannot yet
	// faithfully proxy, so we can reject them loudly instead of silently
	// degrading the conversation. See requestUsesTools.
	Tools      json.RawMessage `json:"tools,omitempty"`
	ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
}

type anthropicInMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"` // string OR array of content blocks
}

// maxAccumulatedStreamBytes caps how much streamed output is buffered in memory
// for policy scanning, bounding both memory and CheckOutput cost per request.
const maxAccumulatedStreamBytes = 1 << 20 // 1 MiB

// mapStopReason translates an OpenAI-style finish_reason into the Anthropic
// Messages stop_reason vocabulary. tool_use is intentionally not produced here
// because tool requests are rejected up front (see requestUsesTools).
func mapStopReason(finishReason string) string {
	switch finishReason {
	case "length":
		return "max_tokens"
	case "content_filter":
		return "end_turn"
	case "stop", "":
		return "end_turn"
	default:
		return "end_turn"
	}
}

// runeTruncate returns at most n bytes of s without splitting a UTF-8 rune.
func runeTruncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// requestUsesTools reports whether the request relies on tool use, which this
// gateway does not yet proxy end to end. It checks the top-level tools/
// tool_choice fields and scans message content for tool_use / tool_result
// blocks. Returns a short reason for the rejection message.
func requestUsesTools(in *anthropicMessagesRequest) (bool, string) {
	if len(in.Tools) > 0 && string(in.Tools) != "null" {
		return true, "tools"
	}
	if len(in.ToolChoice) > 0 && string(in.ToolChoice) != "null" {
		return true, "tool_choice"
	}
	for _, m := range in.Messages {
		var blocks []struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(m.Content, &blocks); err != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "tool_use" || b.Type == "tool_result" {
				return true, b.Type + " content block"
			}
		}
	}
	return false, ""
}

// --- Outbound response types (Anthropic Messages API) ---

type anthropicTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicMsgUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicMessagesResponse struct {
	ID           string               `json:"id"`
	Type         string               `json:"type"`
	Role         string               `json:"role"`
	Model        string               `json:"model"`
	Content      []anthropicTextBlock `json:"content"`
	StopReason   string               `json:"stop_reason"`
	StopSequence *string              `json:"stop_sequence"`
	Usage        anthropicMsgUsage    `json:"usage"`
}

type anthropicErrorEnvelope struct {
	Type  string             `json:"type"`
	Error anthropicErrorBody `json:"error"`
}

type anthropicErrorBody struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// flattenAnthropicContent reduces an Anthropic content value — which may be a
// plain string or an array of typed blocks — to a single text string. Text
// blocks are concatenated; non-text blocks are ignored.
func flattenAnthropicContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Case 1: plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Case 2: array of blocks.
	var blocks []anthropicTextBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "")
	}
	return ""
}

// translateMessagesRequest converts an Anthropic Messages request into the
// internal ChatCompletionRequest the rest of the pipeline understands.
func translateMessagesRequest(in *anthropicMessagesRequest) *types.ChatCompletionRequest {
	msgs := make([]types.Message, 0, len(in.Messages)+1)
	if sys := flattenAnthropicContent(in.System); sys != "" {
		msgs = append(msgs, types.Message{Role: "system", Content: sys})
	}
	for _, m := range in.Messages {
		msgs = append(msgs, types.Message{Role: m.Role, Content: flattenAnthropicContent(m.Content)})
	}
	out := &types.ChatCompletionRequest{
		Model:       in.Model,
		Messages:    msgs,
		Stream:      in.Stream,
		Temperature: in.Temperature,
		TopP:        in.TopP,
		Stop:        in.StopSequences,
	}
	if in.MaxTokens > 0 {
		mt := in.MaxTokens
		out.MaxTokens = &mt
	}
	return out
}

func newAnthropicMsgID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "msg_aegisflow"
	}
	return "msg_" + hex.EncodeToString(b)
}

// auditDetail builds a valid JSON object string for the audit log detail
// field. Using json.Marshal (rather than string concatenation) keeps the
// record well-formed even when the prompt or policy message contains double
// quotes or backslashes.
func auditDetail(fields map[string]string) string {
	b, err := json.Marshal(fields)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func writeAnthropicError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(anthropicErrorEnvelope{
		Type:  "error",
		Error: anthropicErrorBody{Type: errType, Message: message},
	})
}

// Messages handles POST /v1/messages (Anthropic Messages API).
func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	var in anthropicMessagesRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, h.maxBodySize)).Decode(&in); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "failed to parse request body")
		return
	}
	if in.Model == "" {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if len(in.Messages) == 0 {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}
	// Reject tool use loudly rather than silently dropping it. This gateway
	// translates text turns only; silently flattening tool_use/tool_result
	// blocks would corrupt an agentic conversation without the client noticing.
	if used, what := requestUsesTools(&in); used {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error",
			"tool use ("+what+") is not yet supported by the AegisFlow /v1/messages endpoint; use a text-only request")
		return
	}

	req := translateMessagesRequest(&in)

	tenantID := ""
	if t := middleware.TenantFromContext(r.Context()); t != nil {
		tenantID = t.ID
	}

	// Apply the same model aliasing + request transforms the OpenAI path uses,
	// so configured governance (system-prompt injection, aliases, per-tenant
	// overrides) applies consistently regardless of wire format.
	if h.modelAliases != nil {
		ApplyModelAlias(req, h.modelAliases)
	}
	var tenantTransform *TransformConfig
	if tenantID != "" && h.tenantTransforms != nil {
		tenantTransform = h.tenantTransforms[tenantID]
	}
	TransformRequestWithTenant(req, h.transformCfg, tenantTransform)

	// Input policy + audit, before anything leaves for the provider.
	if h.policy != nil {
		inputContent := extractContent(req.Messages)
		v, err := h.policy.CheckInput(inputContent)
		if err != nil {
			log.Printf("policy engine input check error: %v", err)
			h.recordAnalytics(tenantID, req.Model, "", http.StatusInternalServerError, startTime, 0)
			writeAnthropicError(w, http.StatusInternalServerError, "api_error", "policy engine error")
			return
		}
		if v != nil {
			if v.Action == policy.ActionBlock {
				h.fireWebhook("policy_violation", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
				h.recordAnalytics(tenantID, req.Model, "", http.StatusForbidden, startTime, 0)
				if h.auditLog != nil {
					prompt := runeTruncate(inputContent, 200)
					h.auditLog("system", "system", "policy.block", "policy:"+v.PolicyName, auditDetail(map[string]string{"message": v.Message, "prompt": prompt, "api": "messages"}), tenantID, req.Model)
				}
				writeAnthropicError(w, http.StatusForbidden, "permission_error", policy.FormatViolation(v))
				return
			}
			h.fireWebhook("policy_warning", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
			log.Printf("policy warning: %s", policy.FormatViolation(v))
		}
	}

	if req.Stream {
		h.messagesStream(w, r, req, in.Model, tenantID, startTime)
		return
	}

	routed, err := h.router.RouteWithProvider(r.Context(), req)
	if err != nil {
		log.Printf("messages: provider routing error: %v", err)
		h.recordAnalytics(tenantID, req.Model, "", http.StatusBadGateway, startTime, 0)
		writeAnthropicError(w, http.StatusBadGateway, "api_error", "upstream provider error")
		return
	}
	resp := routed.Response
	providerName := routed.Provider

	// Output policy.
	if h.policy != nil && len(resp.Choices) > 0 {
		v, err := h.policy.CheckOutput(resp.Choices[0].Message.Content)
		if err != nil {
			log.Printf("policy engine output check error: %v", err)
			writeAnthropicError(w, http.StatusInternalServerError, "api_error", "policy engine error")
			return
		}
		if v != nil && v.Action == policy.ActionBlock {
			h.fireWebhook("policy_violation", v.PolicyName, string(v.Action), tenantID, req.Model, v.Message)
			h.recordAnalytics(tenantID, req.Model, providerName, http.StatusForbidden, startTime, 0)
			if h.auditLog != nil {
				h.auditLog("system", "system", "policy.block", "policy:"+v.PolicyName, auditDetail(map[string]string{"message": v.Message, "phase": "output", "api": "messages"}), tenantID, req.Model)
			}
			writeAnthropicError(w, http.StatusForbidden, "permission_error", policy.FormatViolation(v))
			return
		}
	}

	if h.usage != nil {
		h.usage.Record(tenantID, providerName, req.Model, resp.Usage)
	}
	h.recordAnalytics(tenantID, req.Model, providerName, http.StatusOK, startTime, int64(resp.Usage.TotalTokens))
	h.logRequest(startTime, r, tenantID, req.Model, providerName, http.StatusOK, resp.Usage.TotalTokens, false, routed.Region)

	content := ""
	finishReason := ""
	if len(resp.Choices) > 0 {
		content = resp.Choices[0].Message.Content
		finishReason = resp.Choices[0].FinishReason
	}
	out := anthropicMessagesResponse{
		ID:         newAnthropicMsgID(),
		Type:       "message",
		Role:       "assistant",
		Model:      in.Model,
		Content:    []anthropicTextBlock{{Type: "text", Text: content}},
		StopReason: mapStopReason(finishReason),
		Usage: anthropicMsgUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// CountTokens handles POST /v1/messages/count_tokens. The Anthropic SDK and
// Claude Code call it to budget context-window usage. AegisFlow has no
// upstream tokenizer, so it returns a byte-based estimate; the value is
// advisory. Response shape: {"input_tokens": N}.
func (h *Handler) CountTokens(w http.ResponseWriter, r *http.Request) {
	var in anthropicMessagesRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, h.maxBodySize)).Decode(&in); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "failed to parse request body")
		return
	}
	if len(in.Messages) == 0 && len(in.System) == 0 {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}
	req := translateMessagesRequest(&in)
	tokens := estimateTokens(extractContent(req.Messages))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"input_tokens": tokens})
}

// writeSSE writes one Anthropic SSE event (named event + JSON data) and flushes.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, payload any) {
	data, _ := json.Marshal(payload)
	w.Write([]byte("event: " + event + "\n"))
	w.Write([]byte("data: "))
	w.Write(data)
	w.Write([]byte("\n\n"))
	flusher.Flush()
}

// messagesStream proxies a streaming completion and re-frames the provider's
// OpenAI-format SSE chunks as Anthropic Messages stream events.
//
// Output policy is enforced check-before-release: incoming deltas are buffered
// and scanned BEFORE being flushed to the client, so violating content never
// egresses. A block emits a terminal Anthropic error event (and nothing else).
// The policy scan runs over a bounded sliding window so memory and CheckOutput
// cost stay linear regardless of total stream length.
func (h *Handler) messagesStream(w http.ResponseWriter, r *http.Request, req *types.ChatCompletionRequest, model, tenantID string, startTime time.Time) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAnthropicError(w, http.StatusInternalServerError, "api_error", "streaming not supported")
		return
	}

	stream, err := h.router.RouteStream(r.Context(), req)
	if err != nil {
		log.Printf("messages stream: provider routing error: %v", err)
		writeAnthropicError(w, http.StatusBadGateway, "api_error", "upstream provider error")
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	msgID := newAnthropicMsgID()
	inputTokens := estimateTokens(extractContent(req.Messages))

	writeSSE(w, flusher, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": msgID, "type": "message", "role": "assistant", "model": model,
			"content": []any{}, "stop_reason": nil, "stop_sequence": nil,
			"usage": map[string]int{"input_tokens": inputTokens, "output_tokens": 0},
		},
	})
	writeSSE(w, flusher, "content_block_start", map[string]any{
		"type": "content_block_start", "index": 0,
		"content_block": map[string]any{"type": "text", "text": ""},
	})

	// scanWindow holds a bounded tail of output for policy scanning; totalOut
	// tracks the full output length for the usage estimate. pending holds
	// deltas scanned-but-not-yet-released so we never egress before checking.
	var scanWindow strings.Builder
	totalOut := 0
	var pending strings.Builder
	pendingBytes := 0
	const checkInterval = 500
	finishReason := ""

	// flushPending scans the current window and, unless blocked, releases the
	// buffered deltas to the client. Returns false if the stream was blocked
	// (caller must stop). The error event, if any, is already written.
	flushPending := func() (ok bool) {
		if pending.Len() == 0 {
			return true
		}
		if h.policy != nil {
			if v, checkErr := h.policy.CheckOutput(scanWindow.String()); checkErr != nil {
				log.Printf("policy engine stream check error: %v", checkErr)
			} else if v != nil && v.Action == policy.ActionBlock {
				h.fireWebhook("stream_policy_violation", v.PolicyName, string(v.Action), tenantID, model, v.Message)
				// Anthropic error events are terminal — nothing follows.
				writeSSE(w, flusher, "error", anthropicErrorEnvelope{
					Type:  "error",
					Error: anthropicErrorBody{Type: "permission_error", Message: v.Message},
				})
				log.Printf("stream terminated: %s", policy.FormatViolation(v))
				return false
			}
		}
		writeSSE(w, flusher, "content_block_delta", map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": pending.String()},
		})
		pending.Reset()
		pendingBytes = 0
		return true
	}

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		var chunk types.StreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		// Capture the finish reason even on the terminating empty-delta chunk.
		if fr := chunk.Choices[0].FinishReason; fr != "" {
			finishReason = fr
		}
		delta := chunk.Choices[0].Delta.Content
		if delta == "" {
			continue
		}

		pending.WriteString(delta)
		pendingBytes += len(delta)
		totalOut += len(delta)

		// Maintain a bounded sliding window for the policy scan.
		scanWindow.WriteString(delta)
		if scanWindow.Len() > maxAccumulatedStreamBytes {
			s := scanWindow.String()
			scanWindow.Reset()
			scanWindow.WriteString(s[len(s)-maxAccumulatedStreamBytes:])
		}

		if pendingBytes >= checkInterval {
			if !flushPending() {
				return
			}
		}
	}

	// Surface a truncated/aborted upstream stream as an error rather than a
	// clean completion.
	if err := scanner.Err(); err != nil {
		log.Printf("messages stream: read error: %v", err)
		writeSSE(w, flusher, "error", anthropicErrorEnvelope{
			Type:  "error",
			Error: anthropicErrorBody{Type: "api_error", Message: "upstream stream error"},
		})
		h.recordAnalytics(tenantID, model, "", http.StatusBadGateway, startTime, int64(totalOut))
		return
	}

	// Final scan + release of the sub-threshold tail.
	if !flushPending() {
		return
	}

	writeSSE(w, flusher, "content_block_stop", map[string]any{
		"type": "content_block_stop", "index": 0,
	})
	outTokens := estimateTokensFromBytes(totalOut)
	writeSSE(w, flusher, "message_delta", map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": mapStopReason(finishReason), "stop_sequence": nil},
		"usage": map[string]int{"input_tokens": inputTokens, "output_tokens": outTokens},
	})
	writeSSE(w, flusher, "message_stop", map[string]any{"type": "message_stop"})

	h.recordAnalytics(tenantID, model, "", http.StatusOK, startTime, int64(outTokens))
}

// estimateTokens is a rough byte-to-token approximation used only for the
// streaming usage field, which Anthropic clients treat as advisory.
func estimateTokens(s string) int {
	return estimateTokensFromBytes(len(s))
}

func estimateTokensFromBytes(n int) int {
	if n <= 0 {
		return 0
	}
	return n/4 + 1
}
