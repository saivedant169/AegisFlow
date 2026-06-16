package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/saivedant169/AegisFlow/internal/httpx"
	"os"
	"time"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

type AnthropicProvider struct {
	name    string
	baseURL string
	apiKey  string
	models  []string
	client  *http.Client
}

type anthropicRequest struct {
	Model      string             `json:"model"`
	MaxTokens  int                `json:"max_tokens"`
	Messages   []anthropicMessage `json:"messages"`
	Tools      []anthropicReqTool `json:"tools,omitempty"`
	ToolChoice json.RawMessage    `json:"tool_choice,omitempty"`
	Stream     bool               `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string, or []anthropicReqBlock when tools are involved
}

// anthropicReqTool is a tool definition in Anthropic's native request shape.
type anthropicReqTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

// anthropicReqBlock is a request content block (text, tool_use, or tool_result).
type anthropicReqBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`        // text
	ID        string          `json:"id,omitempty"`          // tool_use
	Name      string          `json:"name,omitempty"`        // tool_use
	Input     json.RawMessage `json:"input,omitempty"`       // tool_use
	ToolUseID string          `json:"tool_use_id,omitempty"` // tool_result
	Content   string          `json:"content,omitempty"`     // tool_result
}

type anthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      anthropicUsage          `json:"usage"`
}

type anthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`    // tool_use
	Name  string          `json:"name,omitempty"`  // tool_use
	Input json.RawMessage `json:"input,omitempty"` // tool_use
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func NewAnthropicProvider(name, baseURL, apiKeyEnv string, models []string, timeout time.Duration) *AnthropicProvider {
	apiKey := os.Getenv(apiKeyEnv)
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	return &AnthropicProvider{
		name:    name,
		baseURL: baseURL,
		apiKey:  apiKey,
		models:  models,
		client:  httpx.Client(timeout),
	}
}

func (a *AnthropicProvider) Name() string {
	return a.name
}

func (a *AnthropicProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	anthReq := a.translateRequest(req)

	body, err := json.Marshal(anthReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var anthResp anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&anthResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return a.translateResponse(&anthResp, req.Model), nil
}

func (a *AnthropicProvider) ChatCompletionStream(ctx context.Context, req *types.ChatCompletionRequest) (io.ReadCloser, error) {
	anthReq := a.translateRequest(req)
	anthReq.Stream = true

	body, err := json.Marshal(anthReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return resp.Body, nil
}

func (a *AnthropicProvider) Models(_ context.Context) ([]types.Model, error) {
	models := make([]types.Model, len(a.models))
	for i, m := range a.models {
		models[i] = types.Model{ID: m, Object: "model", Provider: a.name}
	}
	return models, nil
}

func (a *AnthropicProvider) EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}

func (a *AnthropicProvider) Healthy(ctx context.Context) bool {
	return a.apiKey != ""
}

func (a *AnthropicProvider) translateRequest(req *types.ChatCompletionRequest) anthropicRequest {
	msgs := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		switch {
		case m.Role == "system":
			continue
		case m.Role == "tool":
			// Tool result -> a user message carrying a tool_result block.
			msgs = append(msgs, anthropicMessage{Role: "user", Content: []anthropicReqBlock{
				{Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content},
			}})
		case len(m.ToolCalls) > 0:
			// Assistant tool call(s) -> content blocks (optional text + tool_use).
			blocks := make([]anthropicReqBlock, 0, 1+len(m.ToolCalls))
			if m.Content != "" {
				blocks = append(blocks, anthropicReqBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(tc.Function.Arguments)
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, anthropicReqBlock{Type: "tool_use", ID: tc.ID, Name: tc.Function.Name, Input: input})
			}
			msgs = append(msgs, anthropicMessage{Role: m.Role, Content: blocks})
		default:
			msgs = append(msgs, anthropicMessage{Role: m.Role, Content: m.Content})
		}
	}

	maxTokens := 1024
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}

	return anthropicRequest{
		Model:      req.Model,
		MaxTokens:  maxTokens,
		Messages:   msgs,
		Tools:      toolsToAnthropic(req.Tools),
		ToolChoice: toolChoiceToAnthropic(req.ToolChoice),
	}
}

// toolsToAnthropic maps internal tool definitions to Anthropic's native shape.
func toolsToAnthropic(tools []types.Tool) []anthropicReqTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]anthropicReqTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, anthropicReqTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	return out
}

// toolChoiceToAnthropic maps the internal (OpenAI-form) tool_choice to Anthropic.
// "auto"->{auto}, "required"->{any}, {function:{name}}->{tool,name}; "none" and
// unknowns omit the field (Anthropic has no explicit "none").
func toolChoiceToAnthropic(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto":
			return json.RawMessage(`{"type":"auto"}`)
		case "required":
			return json.RawMessage(`{"type":"any"}`)
		}
		return nil
	}
	var obj struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil && obj.Function.Name != "" {
		b, err := json.Marshal(map[string]string{"type": "tool", "name": obj.Function.Name})
		if err == nil {
			return b
		}
	}
	return nil
}

func (a *AnthropicProvider) translateResponse(resp *anthropicResponse, model string) *types.ChatCompletionResponse {
	content := ""
	var toolCalls []types.ToolCall
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			input := string(block.Input)
			if input == "" {
				input = "{}"
			}
			toolCalls = append(toolCalls, types.ToolCall{
				ID:       block.ID,
				Type:     "function",
				Function: types.ToolCallFunction{Name: block.Name, Arguments: input},
			})
		}
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	return &types.ChatCompletionResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []types.Choice{
			{
				Index:        0,
				Message:      types.Message{Role: "assistant", Content: content, ToolCalls: toolCalls},
				FinishReason: finishReason,
			},
		},
		Usage: types.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}
