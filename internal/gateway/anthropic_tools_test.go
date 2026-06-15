package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/policy"
	"github.com/saivedant169/AegisFlow/internal/provider"
	"github.com/saivedant169/AegisFlow/internal/router"
	"github.com/saivedant169/AegisFlow/internal/usage"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

// toolEchoProvider returns a tool call echoing the first tool when the request
// carries tools, standing in for an OpenAI-compatible provider that preserves
// tool semantics. Lets the full /v1/messages tool loop be exercised locally.
type toolEchoProvider struct{}

func (toolEchoProvider) Name() string { return "toolecho" }
func (toolEchoProvider) ChatCompletion(_ context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	if len(req.Tools) > 0 {
		return &types.ChatCompletionResponse{
			Model: req.Model,
			Choices: []types.Choice{{FinishReason: "tool_calls", Message: types.Message{
				Role: "assistant",
				ToolCalls: []types.ToolCall{{ID: "tu_echo", Type: "function",
					Function: types.ToolCallFunction{Name: req.Tools[0].Function.Name, Arguments: `{"echo":true}`}}},
			}}},
			Usage: types.Usage{TotalTokens: 3},
		}, nil
	}
	return &types.ChatCompletionResponse{Model: req.Model, Choices: []types.Choice{{FinishReason: "stop", Message: types.Message{Role: "assistant", Content: "no tools"}}}}, nil
}
func (toolEchoProvider) ChatCompletionStream(context.Context, *types.ChatCompletionRequest) (io.ReadCloser, error) {
	return nil, fmt.Errorf("streaming not supported")
}
func (toolEchoProvider) Models(context.Context) ([]types.Model, error) {
	return []types.Model{{ID: "mock"}}, nil
}
func (toolEchoProvider) EstimateTokens(s string) int  { return len(s) / 4 }
func (toolEchoProvider) Healthy(context.Context) bool { return true }

// Full /v1/messages tool loop: an Anthropic tool request is translated, the
// provider returns a tool call, and the response is emitted as a tool_use block.
func TestMessages_ToolRoundTrip(t *testing.T) {
	registry := provider.NewRegistry()
	registry.Register(toolEchoProvider{})
	routes := []config.RouteConfig{{Match: config.RouteMatch{Model: "*"}, Providers: []string{"toolecho"}, Strategy: "priority"}}
	rt := router.NewRouter(routes, registry)
	h := NewHandler(registry, rt, policy.NewEngine(nil, nil), usage.NewTracker(usage.NewStore()), nil, nil, nil, nil, 0, nil, nil)
	h.SetMessagesToolPassthrough(true)

	w := postMessagesAsTenant(h, "t1", toolReqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var out anthropicMessagesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", out.StopReason)
	}
	found := false
	for _, b := range out.Content {
		if b.Type == "tool_use" && b.Name == "get_weather" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a tool_use block for get_weather, got %+v", out.Content)
	}
}

// A response carrying tool calls must serialize as Anthropic tool_use blocks
// with stop_reason "tool_use".
func TestWriteAnthropicMessage_EmitsToolUse(t *testing.T) {
	w := httptest.NewRecorder()
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{{
			FinishReason: "tool_calls",
			Message: types.Message{
				Role: "assistant",
				ToolCalls: []types.ToolCall{{
					ID: "tu_1", Type: "function",
					Function: types.ToolCallFunction{Name: "get_weather", Arguments: `{"city":"SF"}`},
				}},
			},
		}},
		Usage: types.Usage{PromptTokens: 5, CompletionTokens: 3},
	}
	writeAnthropicMessage(w, "claude-x", resp)

	var out anthropicMessagesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use", out.StopReason)
	}
	var tu *anthropicRespBlock
	for i := range out.Content {
		if out.Content[i].Type == "tool_use" {
			tu = &out.Content[i]
		}
	}
	if tu == nil {
		t.Fatalf("no tool_use block in %+v", out.Content)
	}
	if tu.ID != "tu_1" || tu.Name != "get_weather" || string(tu.Input) != `{"city":"SF"}` {
		t.Errorf("unexpected tool_use block: %+v", tu)
	}
}

// A plain text response still serializes as a single text block.
func TestWriteAnthropicMessage_TextOnly(t *testing.T) {
	w := httptest.NewRecorder()
	resp := &types.ChatCompletionResponse{
		Choices: []types.Choice{{FinishReason: "stop", Message: types.Message{Role: "assistant", Content: "hi there"}}},
	}
	writeAnthropicMessage(w, "claude-x", resp)
	var out anthropicMessagesResponse
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.StopReason != "end_turn" {
		t.Errorf("stop_reason = %q, want end_turn", out.StopReason)
	}
	if len(out.Content) != 1 || out.Content[0].Type != "text" || out.Content[0].Text != "hi there" {
		t.Errorf("unexpected content: %+v", out.Content)
	}
}

func TestTranslateAnthropicMessage_ToolUse(t *testing.T) {
	m := anthropicInMessage{Role: "assistant", Content: []byte(`[
		{"type":"text","text":"let me check"},
		{"type":"tool_use","id":"tu_1","name":"get_weather","input":{"city":"SF"}}
	]`)}
	out := translateAnthropicMessage(m)
	if len(out) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out))
	}
	if out[0].Role != "assistant" || out[0].Content != "let me check" {
		t.Errorf("unexpected role/content: %+v", out[0])
	}
	if len(out[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(out[0].ToolCalls))
	}
	tc := out[0].ToolCalls[0]
	if tc.ID != "tu_1" || tc.Function.Name != "get_weather" {
		t.Errorf("unexpected tool call: %+v", tc)
	}
	if tc.Function.Arguments != `{"city":"SF"}` {
		t.Errorf("arguments = %q", tc.Function.Arguments)
	}
}

func TestTranslateAnthropicMessage_ToolResult(t *testing.T) {
	m := anthropicInMessage{Role: "user", Content: []byte(`[
		{"type":"tool_result","tool_use_id":"tu_1","content":"72F and sunny"}
	]`)}
	out := translateAnthropicMessage(m)
	if len(out) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out))
	}
	if out[0].Role != "tool" || out[0].ToolCallID != "tu_1" || out[0].Content != "72F and sunny" {
		t.Errorf("unexpected tool-result message: %+v", out[0])
	}
}

func TestTranslateAnthropicMessage_PlainString(t *testing.T) {
	m := anthropicInMessage{Role: "user", Content: []byte(`"hello"`)}
	out := translateAnthropicMessage(m)
	if len(out) != 1 || out[0].Role != "user" || out[0].Content != "hello" {
		t.Errorf("plain string translation wrong: %+v", out)
	}
}

func TestTranslateAnthropicTools(t *testing.T) {
	tools := translateAnthropicTools([]byte(`[{"name":"get_weather","description":"d","input_schema":{"type":"object"}}]`))
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Type != "function" || tools[0].Function.Name != "get_weather" || tools[0].Function.Description != "d" {
		t.Errorf("unexpected tool: %+v", tools[0])
	}
	if string(tools[0].Function.Parameters) != `{"type":"object"}` {
		t.Errorf("parameters = %s", tools[0].Function.Parameters)
	}
}

func TestTranslateAnthropicToolChoice(t *testing.T) {
	cases := map[string]string{
		`{"type":"auto"}`:                      `"auto"`,
		`{"type":"none"}`:                      `"none"`,
		`{"type":"any"}`:                       `"required"`,
		`{"type":"tool","name":"get_weather"}`: `{"function":{"name":"get_weather"},"type":"function"}`,
	}
	for in, want := range cases {
		got := string(translateAnthropicToolChoice([]byte(in)))
		if got != want {
			t.Errorf("tool_choice %s => %s, want %s", in, got, want)
		}
	}
}

const toolReqBody = `{"model":"mock","max_tokens":64,"messages":[{"role":"user","content":"weather?"}],"tools":[{"name":"get_weather","description":"get weather","input_schema":{"type":"object"}}]}`

// With the flag off, a tool request is still rejected loudly.
func TestMessages_ToolPassthroughDisabled_Rejects(t *testing.T) {
	h, _ := newMessagesHandler(nil)
	w := postMessagesAsTenant(h, "t1", toolReqBody)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 with tool passthrough off, got %d: %s", w.Code, w.Body.String())
	}
}

// With the flag on, a tool request is translated and served (no 400).
func TestMessages_ToolPassthroughEnabled_Translates(t *testing.T) {
	h, _ := newMessagesHandler(nil)
	h.SetMessagesToolPassthrough(true)
	w := postMessagesAsTenant(h, "t1", toolReqBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with tool passthrough on, got %d: %s", w.Code, w.Body.String())
	}
}
