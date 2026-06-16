package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

// Ollama tool definitions are OpenAI-shaped; tool-call arguments are a JSON
// object on the wire but a string internally. Verify both directions translate.
func TestOllamaProvider_ToolsRoundTrip(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"model":"llama","done":true,
			"message":{"role":"assistant","content":"",
				"tool_calls":[{"function":{"name":"get_weather","arguments":{"city":"SF"}}}]},
			"prompt_eval_count":5,"eval_count":3
		}`))
	}))
	defer srv.Close()

	p := NewOllamaProvider("ollama-test", srv.URL, []string{"llama"})
	resp, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model:    "llama",
		Messages: []types.Message{{Role: "user", Content: "weather in SF?"}},
		Tools:    []types.Tool{{Type: "function", Function: types.ToolFunction{Name: "get_weather", Description: "Get weather", Parameters: json.RawMessage(`{"type":"object"}`)}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tools forwarded (OpenAI shape) in the outbound body.
	if !strings.Contains(gotBody, `"tools"`) || !strings.Contains(gotBody, `"get_weather"`) {
		t.Errorf("tools not forwarded: %s", gotBody)
	}
	// Response tool call surfaced; object arguments stringified.
	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 1 || calls[0].Function.Name != "get_weather" || calls[0].Function.Arguments != `{"city":"SF"}` {
		t.Errorf("tool call not surfaced: %+v", calls)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("finish_reason = %q, want tool_calls", resp.Choices[0].FinishReason)
	}
}

// A prior assistant tool call is sent with object arguments (Ollama's shape).
func TestOllamaMessagesFrom_ToolCallArgsAsObject(t *testing.T) {
	msgs := ollamaMessagesFrom([]types.Message{
		{Role: "assistant", ToolCalls: []types.ToolCall{{Function: types.ToolCallFunction{Name: "f", Arguments: `{"x":1}`}}}},
	})
	b, _ := json.Marshal(msgs[0])
	s := string(b)
	if !strings.Contains(s, `"arguments":{"x":1}`) {
		t.Errorf("arguments must serialize as an object, got: %s", s)
	}
}
