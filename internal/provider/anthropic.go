package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	ID      string                   `json:"id"`
	Type    string                   `json:"type"`
	Role    string                   `json:"role"`
	Content []anthropicContentBlock  `json:"content"`
	Model   string                   `json:"model"`
	Usage   anthropicUsage           `json:"usage"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
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
		client:  &http.Client{Timeout: timeout},
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
		if m.Role == "system" {
			continue
		}
		msgs = append(msgs, anthropicMessage{Role: m.Role, Content: m.Content})
	}

	maxTokens := 1024
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}

	return anthropicRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Messages:  msgs,
	}
}

func (a *AnthropicProvider) translateResponse(resp *anthropicResponse, model string) *types.ChatCompletionResponse {
	content := ""
	for _, block := range resp.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	return &types.ChatCompletionResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []types.Choice{
			{
				Index:        0,
				Message:      types.Message{Role: "assistant", Content: content},
				FinishReason: "stop",
			},
		},
		Usage: types.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}
