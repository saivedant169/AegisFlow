package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

// The OpenAI provider must forward tool definitions to the upstream and surface
// tool calls from the response — previously these were silently dropped because
// the internal types had no tool fields.
func TestOpenAIForwardsToolsAndReturnsToolCalls(t *testing.T) {
	var gotTools []types.Tool
	var gotToolChoice string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req types.ChatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)
		gotTools = req.Tools
		gotToolChoice = string(req.ToolChoice)

		json.NewEncoder(w).Encode(types.ChatCompletionResponse{
			ID:    "chatcmpl-tools",
			Model: "gpt-4o",
			Choices: []types.Choice{{
				Index:        0,
				FinishReason: "tool_calls",
				Message: types.Message{
					Role: "assistant",
					ToolCalls: []types.ToolCall{{
						ID:       "call_1",
						Type:     "function",
						Function: types.ToolCallFunction{Name: "get_weather", Arguments: `{"city":"SF"}`},
					}},
				},
			}},
			Usage: types.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
		})
	}))
	defer srv.Close()

	p := newTestOpenAIProvider(srv, "test-key")
	req := &types.ChatCompletionRequest{
		Model:      "gpt-4o",
		Messages:   []types.Message{{Role: "user", Content: "weather in SF?"}},
		Tools:      []types.Tool{{Type: "function", Function: types.ToolFunction{Name: "get_weather", Parameters: json.RawMessage(`{"type":"object"}`)}}},
		ToolChoice: json.RawMessage(`"auto"`),
	}

	resp, err := p.ChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Forwarded upstream.
	if len(gotTools) != 1 || gotTools[0].Function.Name != "get_weather" {
		t.Errorf("tools not forwarded to upstream: %+v", gotTools)
	}
	if gotToolChoice != `"auto"` {
		t.Errorf("tool_choice not forwarded: %q", gotToolChoice)
	}
	// Surfaced from the response.
	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 1 || calls[0].Function.Name != "get_weather" || calls[0].Function.Arguments != `{"city":"SF"}` {
		t.Errorf("tool calls not surfaced from response: %+v", calls)
	}
}
