package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/saivedant169/AegisFlow/internal/httpx"
	"time"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

type OllamaProvider struct {
	name    string
	baseURL string
	models  []string
	client  *http.Client
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Tools    []types.Tool    `json:"tools,omitempty"` // Ollama tool definitions are OpenAI-shaped
	Stream   bool            `json:"stream"`
}

type ollamaMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

// ollamaToolCall mirrors Ollama's tool call shape. Unlike OpenAI, Ollama carries
// arguments as a JSON object, not a JSON-encoded string, so Arguments is raw.
type ollamaToolCall struct {
	Function ollamaToolCallFunc `json:"function"`
}

type ollamaToolCallFunc struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ollamaMessagesFrom translates internal messages to Ollama messages, carrying
// assistant tool calls (arguments as an object) and tool results.
func ollamaMessagesFrom(msgs []types.Message) []ollamaMessage {
	out := make([]ollamaMessage, 0, len(msgs))
	for _, m := range msgs {
		om := ollamaMessage{Role: m.Role, Content: m.Content}
		for _, tc := range m.ToolCalls {
			args := json.RawMessage(tc.Function.Arguments)
			if len(args) == 0 {
				args = json.RawMessage("{}")
			}
			om.ToolCalls = append(om.ToolCalls, ollamaToolCall{Function: ollamaToolCallFunc{Name: tc.Function.Name, Arguments: args}})
		}
		out = append(out, om)
	}
	return out
}

type ollamaChatResponse struct {
	Model           string        `json:"model"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}

func NewOllamaProvider(name, baseURL string, models []string) *OllamaProvider {
	return &OllamaProvider{
		name:    name,
		baseURL: baseURL,
		models:  models,
		client:  httpx.Client(120 * time.Second),
	}
}

func (o *OllamaProvider) Name() string {
	return o.name
}

func (o *OllamaProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	ollamaReq := ollamaChatRequest{
		Model:    req.Model,
		Messages: ollamaMessagesFrom(req.Messages),
		Tools:    req.Tools,
		Stream:   false,
	}

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	content := ollamaResp.Message.Content
	promptTokens := ollamaResp.PromptEvalCount
	completionTokens := ollamaResp.EvalCount
	if promptTokens == 0 {
		promptTokens = o.EstimateTokens(req.Messages[len(req.Messages)-1].Content)
	}
	if completionTokens == 0 {
		completionTokens = o.EstimateTokens(content)
	}

	var toolCalls []types.ToolCall
	for _, oc := range ollamaResp.Message.ToolCalls {
		toolCalls = append(toolCalls, types.ToolCall{
			Type:     "function",
			Function: types.ToolCallFunction{Name: oc.Function.Name, Arguments: string(oc.Function.Arguments)},
		})
	}
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	return &types.ChatCompletionResponse{
		ID:      fmt.Sprintf("aegis-ollama-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []types.Choice{
			{
				Index:        0,
				Message:      types.Message{Role: "assistant", Content: content, ToolCalls: toolCalls},
				FinishReason: finishReason,
			},
		},
		Usage: types.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
	}, nil
}

func (o *OllamaProvider) ChatCompletionStream(ctx context.Context, req *types.ChatCompletionRequest) (io.ReadCloser, error) {
	ollamaReq := ollamaChatRequest{
		Model:    req.Model,
		Messages: ollamaMessagesFrom(req.Messages),
		Tools:    req.Tools,
		Stream:   true,
	}

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Ollama streams NDJSON, but OpenAI SDKs expect SSE format.
	// Convert: {"message":{"content":"hi"},"done":false}
	// To:      data: {"choices":[{"delta":{"content":"hi"}}]}\n\n
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		id := fmt.Sprintf("aegis-ollama-%d", time.Now().UnixNano())

		for decoder.More() {
			var chunk ollamaChatResponse
			if err := decoder.Decode(&chunk); err != nil {
				break
			}

			if chunk.Done {
				finalChunk := types.StreamChunk{
					ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: req.Model,
					Choices: []types.StreamDelta{{Index: 0, Delta: types.Delta{}, FinishReason: "stop"}},
				}
				data, _ := json.Marshal(finalChunk)
				fmt.Fprintf(pw, "data: %s\n\n", data)
				fmt.Fprint(pw, "data: [DONE]\n\n")
				break
			}

			sseChunk := types.StreamChunk{
				ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: req.Model,
				Choices: []types.StreamDelta{{Index: 0, Delta: types.Delta{Content: chunk.Message.Content}}},
			}
			data, _ := json.Marshal(sseChunk)
			fmt.Fprintf(pw, "data: %s\n\n", data)
		}
	}()

	return pr, nil
}

func (o *OllamaProvider) Models(ctx context.Context) ([]types.Model, error) {
	// Try to fetch live models from Ollama
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/tags", nil)
	if err == nil {
		resp, err := o.client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			var result struct {
				Models []struct {
					Name string `json:"name"`
				} `json:"models"`
			}
			if json.NewDecoder(resp.Body).Decode(&result) == nil && len(result.Models) > 0 {
				models := make([]types.Model, len(result.Models))
				for i, m := range result.Models {
					models[i] = types.Model{ID: m.Name, Object: "model", Provider: o.name}
				}
				return models, nil
			}
		}
	}

	// Fallback to configured models
	models := make([]types.Model, len(o.models))
	for i, m := range o.models {
		models[i] = types.Model{ID: m, Object: "model", Provider: o.name}
	}
	return models, nil
}

func (o *OllamaProvider) EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}

func (o *OllamaProvider) Healthy(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
