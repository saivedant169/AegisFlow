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

func newTestGeminiToolProvider(srv *httptest.Server) *GeminiProvider {
	return &GeminiProvider{
		name:    "gemini-test",
		baseURL: srv.URL,
		apiKey:  "k",
		models:  []string{"gemini-x"},
		client:  srv.Client(),
	}
}

// Gemini forwards tools as functionDeclarations + toolConfig, and returns tool
// calls as functionCall parts (args as an object). Verify both directions.
func TestGeminiProvider_ToolsRoundTrip(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[
				{"functionCall":{"name":"get_weather","args":{"city":"SF"}}}
			]}}],
			"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,"totalTokenCount":8}
		}`))
	}))
	defer srv.Close()

	p := newTestGeminiToolProvider(srv)
	resp, err := p.ChatCompletion(context.Background(), &types.ChatCompletionRequest{
		Model:      "gemini-x",
		Messages:   []types.Message{{Role: "user", Content: "weather in SF?"}},
		Tools:      []types.Tool{{Type: "function", Function: types.ToolFunction{Name: "get_weather", Description: "Get weather", Parameters: json.RawMessage(`{"type":"object"}`)}}},
		ToolChoice: json.RawMessage(`"auto"`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{`"functionDeclarations"`, `"get_weather"`, `"toolConfig"`, `"mode":"AUTO"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("outbound request missing %s: %s", want, gotBody)
		}
	}

	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) != 1 || calls[0].Function.Name != "get_weather" || calls[0].Function.Arguments != `{"city":"SF"}` {
		t.Errorf("functionCall not surfaced as tool call: %+v", calls)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Errorf("finish_reason = %q, want tool_calls", resp.Choices[0].FinishReason)
	}
}

func TestGeminiToolConfigFrom(t *testing.T) {
	cases := map[string]string{
		`"auto"`:     "AUTO",
		`"required"`: "ANY",
		`"none"`:     "NONE",
		`{"type":"function","function":{"name":"f"}}`: "ANY",
	}
	for in, wantMode := range cases {
		cfg := geminiToolConfigFrom(json.RawMessage(in))
		if cfg == nil || cfg.FunctionCallingConfig == nil || cfg.FunctionCallingConfig.Mode != wantMode {
			t.Errorf("tool_choice %s => %+v, want mode %s", in, cfg, wantMode)
		}
	}
}

// A tool result (referenced by id internally) is emitted as a Gemini
// functionResponse keyed by the function's name with an object response.
func TestGeminiTranslate_ToolResultByName(t *testing.T) {
	p := &GeminiProvider{}
	gem := p.translateRequest(&types.ChatCompletionRequest{
		Messages: []types.Message{
			{Role: "assistant", ToolCalls: []types.ToolCall{{ID: "c1", Function: types.ToolCallFunction{Name: "get_weather", Arguments: `{"city":"SF"}`}}}},
			{Role: "tool", ToolCallID: "c1", Content: "72F"},
		},
	})
	raw, _ := json.Marshal(gem)
	s := string(raw)
	if !strings.Contains(s, `"functionCall"`) || !strings.Contains(s, `"functionResponse"`) {
		t.Errorf("expected functionCall + functionResponse parts: %s", s)
	}
	if !strings.Contains(s, `"name":"get_weather"`) || !strings.Contains(s, `"result":"72F"`) {
		t.Errorf("functionResponse should key by name with wrapped result: %s", s)
	}
}
