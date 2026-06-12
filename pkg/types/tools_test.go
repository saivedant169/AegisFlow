package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestMarshalsTools(t *testing.T) {
	req := ChatCompletionRequest{
		Model:      "gpt-4o",
		Messages:   []Message{{Role: "user", Content: "weather in SF?"}},
		Tools:      []Tool{{Type: "function", Function: ToolFunction{Name: "get_weather", Description: "Get weather", Parameters: json.RawMessage(`{"type":"object"}`)}}},
		ToolChoice: json.RawMessage(`"auto"`),
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, want := range []string{`"tools"`, `"get_weather"`, `"tool_choice":"auto"`, `"parameters":{"type":"object"}`} {
		if !strings.Contains(s, want) {
			t.Errorf("marshaled request missing %s: %s", want, s)
		}
	}
}

func TestRequestOmitsToolsWhenAbsent(t *testing.T) {
	b, _ := json.Marshal(ChatCompletionRequest{Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}})
	if strings.Contains(string(b), "tools") || strings.Contains(string(b), "tool_choice") {
		t.Errorf("tool fields should be omitted when unset: %s", b)
	}
}

func TestResponseUnmarshalsToolCalls(t *testing.T) {
	raw := `{
      "id":"chatcmpl-1","object":"chat.completion","model":"gpt-4o",
      "choices":[{"index":0,"finish_reason":"tool_calls","message":{
        "role":"assistant","content":"",
        "tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"SF\"}"}}]
      }}],
      "usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}
    }`
	var resp ChatCompletionResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice")
	}
	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].ID != "call_1" || calls[0].Function.Name != "get_weather" {
		t.Errorf("unexpected tool call: %+v", calls[0])
	}
	if calls[0].Function.Arguments != `{"city":"SF"}` {
		t.Errorf("arguments = %q", calls[0].Function.Arguments)
	}
}

func TestToolResultMessageRoundTrip(t *testing.T) {
	m := Message{Role: "tool", Content: "72F and sunny", ToolCallID: "call_1"}
	b, _ := json.Marshal(m)
	var got Message
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ToolCallID != "call_1" || got.Content != "72F and sunny" || got.Role != "tool" {
		t.Errorf("round-trip lost data: %+v", got)
	}
}
