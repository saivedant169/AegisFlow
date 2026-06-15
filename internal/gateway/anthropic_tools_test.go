package gateway

import (
	"net/http"
	"testing"
)

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
