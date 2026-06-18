package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func newTestAnthropicProvider(srv *httptest.Server) *AnthropicProvider {
	return &AnthropicProvider{
		name:    "anthropic-test",
		baseURL: srv.URL,
		apiKey:  "test-key",
		models:  []string{"claude-x"},
		client:  srv.Client(),
	}
}

// The Anthropic provider must forward tool definitions in Anthropic's native
// shape and surface tool_use blocks from the response as internal tool calls.
func TestAnthropicProvider_ToolsRoundTrip(t *testing.T) {
	var got anthropicRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id":"msg_1","type":"message","role":"assistant","model":"claude-x",
			"stop_reason":"tool_use",
			"content":[
				{"type":"text","text":"let me check"},
				{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"SF"}}
			],
			"usage":{"input_tokens":5,"output_tokens":3}
		}`))
	}))
	defer srv.Close()

	p := newTestAnthropicProvider(srv)
	resp, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model:      "claude-x",
		Messages:   []types.Message{{Role: "user", Content: "weather in SF?"}},
		Tools:      []types.Tool{{Type: "function", Function: types.ToolFunction{Name: "get_weather", Description: "Get weather", Parameters: json.RawMessage(`{"type":"object"}`)}}},
		ToolChoice: json.RawMessage(`"auto"`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Forwarded in native shape.
	if len(got.Tools) != 1 || got.Tools[0].Name != "get_weather" || string(got.Tools[0].InputSchema) != `{"type":"object"}` {
		t.Errorf("tools not forwarded in Anthropic shape: %+v", got.Tools)
	}
	if string(got.ToolChoice) != `{"type":"auto"}` {
		t.Errorf("tool_choice not bridged to Anthropic shape: %s", got.ToolChoice)
	}

	// Surfaced from the response.
	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 1 || calls[0].ID != "toolu_1" || calls[0].Function.Name != "get_weather" || calls[0].Function.Arguments != `{"city":"SF"}` {
		t.Errorf("tool_use not surfaced as tool calls: %+v", calls)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("finish_reason = %q, want tool_calls", resp.Choices[0].FinishReason)
	}
}

// System messages must reach Anthropic as the top-level system field, not be
// dropped (which silently lost injected system prompts).
func TestAnthropicProvider_SystemPromptForwarded(t *testing.T) {
	var got anthropicRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	p := newTestAnthropicProvider(srv)
	_, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model: "claude-x",
		Messages: []types.Message{
			{Role: "system", Content: "You are governed."},
			{Role: "user", Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.System != "You are governed." {
		t.Errorf("system prompt not forwarded as top-level field: %q", got.System)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" {
		t.Errorf("system message must not appear in messages: %+v", got.Messages)
	}
}

// An assistant tool call + a tool result must serialize into Anthropic tool_use
// and tool_result content blocks.
func TestAnthropicProvider_ToolMessagesToBlocks(t *testing.T) {
	var got anthropicRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	p := newTestAnthropicProvider(srv)
	_, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model: "claude-x",
		Messages: []types.Message{
			{Role: "user", Content: "weather?"},
			{Role: "assistant", ToolCalls: []types.ToolCall{{ID: "toolu_1", Type: "function", Function: types.ToolCallFunction{Name: "get_weather", Arguments: `{"city":"SF"}`}}}},
			{Role: "tool", ToolCallID: "toolu_1", Content: "72F"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Re-marshal the captured request and inspect the block structure.
	raw, _ := json.Marshal(got)
	var probe struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	json.Unmarshal(raw, &probe)
	if len(probe.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(probe.Messages))
	}
	// assistant message content must be a tool_use block array.
	if s := string(probe.Messages[1].Content); !strings.Contains(s, `"type":"tool_use"`) || !strings.Contains(s, `"id":"toolu_1"`) {
		t.Errorf("assistant tool_use block wrong: %s", s)
	}
	// tool message -> user role with tool_result block.
	if probe.Messages[2].Role != "user" || !strings.Contains(string(probe.Messages[2].Content), `"type":"tool_result"`) || !strings.Contains(string(probe.Messages[2].Content), `"tool_use_id":"toolu_1"`) {
		t.Errorf("tool_result block wrong: role=%s content=%s", probe.Messages[2].Role, probe.Messages[2].Content)
	}
}
