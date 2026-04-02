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

// GeminiProvider handles Google Gemini API.
// Gemini uses a different request/response format than OpenAI.
// URL: https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent?key={apiKey}
// Streaming: models/{model}:streamGenerateContent?key={apiKey}&alt=sse

type GeminiProvider struct {
	name    string
	baseURL string
	apiKey  string
	models  []string
	client  *http.Client
}

type geminiRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"maxOutputTokens,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
}

type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage   `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type geminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func NewGeminiProvider(name, apiKeyEnv string, models []string, timeout time.Duration) *GeminiProvider {
	apiKey := os.Getenv(apiKeyEnv)
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	if len(models) == 0 {
		models = []string{"gemini-2.0-flash", "gemini-1.5-pro"}
	}
	return &GeminiProvider{
		name:    name,
		baseURL: "https://generativelanguage.googleapis.com/v1beta",
		apiKey:  apiKey,
		models:  models,
		client:  &http.Client{Timeout: timeout},
	}
}

func (g *GeminiProvider) Name() string {
	return g.name
}

func (g *GeminiProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	gemReq := g.translateRequest(req)

	body, err := json.Marshal(gemReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", g.baseURL, req.Model, g.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var gemResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gemResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return g.translateResponse(&gemResp, req.Model), nil
}

func (g *GeminiProvider) ChatCompletionStream(ctx context.Context, req *types.ChatCompletionRequest) (io.ReadCloser, error) {
	gemReq := g.translateRequest(req)

	body, err := json.Marshal(gemReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?key=%s&alt=sse", g.baseURL, req.Model, g.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Gemini streaming with alt=sse returns SSE format but with Gemini's JSON structure.
	// Convert to OpenAI SSE format.
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		defer resp.Body.Close()

		id := fmt.Sprintf("aegis-gemini-%d", time.Now().UnixNano())
		buf := make([]byte, 4096)

		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				// Parse Gemini SSE and re-emit as OpenAI SSE
				// For simplicity, relay the raw SSE and let the client handle it
				// In production, you'd parse each "data: {...}" line and translate
				line := string(buf[:n])

				// Try to extract text from Gemini chunk
				var gemResp geminiResponse
				// Strip "data: " prefix if present
				dataStr := line
				if len(dataStr) > 6 && dataStr[:6] == "data: " {
					dataStr = dataStr[6:]
				}
				if json.Unmarshal([]byte(dataStr), &gemResp) == nil && len(gemResp.Candidates) > 0 {
					content := ""
					for _, part := range gemResp.Candidates[0].Content.Parts {
						content += part.Text
					}
					chunk := types.StreamChunk{
						ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: req.Model,
						Choices: []types.StreamDelta{{Index: 0, Delta: types.Delta{Content: content}}},
					}
					data, _ := json.Marshal(chunk)
					fmt.Fprintf(pw, "data: %s\n\n", data)
				}
			}
			if err == io.EOF {
				// Send final chunk
				finalChunk := types.StreamChunk{
					ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: req.Model,
					Choices: []types.StreamDelta{{Index: 0, Delta: types.Delta{}, FinishReason: "stop"}},
				}
				data, _ := json.Marshal(finalChunk)
				fmt.Fprintf(pw, "data: %s\n\n", data)
				fmt.Fprint(pw, "data: [DONE]\n\n")
				break
			}
			if err != nil {
				break
			}
		}
	}()

	return pr, nil
}

func (g *GeminiProvider) Models(_ context.Context) ([]types.Model, error) {
	models := make([]types.Model, len(g.models))
	for i, m := range g.models {
		models[i] = types.Model{ID: m, Object: "model", Provider: g.name}
	}
	return models, nil
}

func (g *GeminiProvider) EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}

func (g *GeminiProvider) Healthy(ctx context.Context) bool {
	return g.apiKey != ""
}

func (g *GeminiProvider) translateRequest(req *types.ChatCompletionRequest) geminiRequest {
	var contents []geminiContent
	for _, m := range req.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		if role == "system" {
			// Gemini handles system as a user message with context
			role = "user"
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Content}},
		})
	}

	gemReq := geminiRequest{Contents: contents}

	if req.Temperature != nil || req.MaxTokens != nil || req.TopP != nil {
		gemReq.GenerationConfig = &geminiGenerationConfig{
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
			TopP:        req.TopP,
		}
	}

	return gemReq
}

func (g *GeminiProvider) translateResponse(resp *geminiResponse, model string) *types.ChatCompletionResponse {
	content := ""
	finishReason := "stop"
	if len(resp.Candidates) > 0 {
		for _, part := range resp.Candidates[0].Content.Parts {
			content += part.Text
		}
		if resp.Candidates[0].FinishReason != "" {
			finishReason = "stop" // normalize Gemini's STOP to OpenAI's stop
		}
	}

	usage := types.Usage{}
	if resp.UsageMetadata != nil {
		usage.PromptTokens = resp.UsageMetadata.PromptTokenCount
		usage.CompletionTokens = resp.UsageMetadata.CandidatesTokenCount
		usage.TotalTokens = resp.UsageMetadata.TotalTokenCount
	}

	return &types.ChatCompletionResponse{
		ID:      fmt.Sprintf("aegis-gemini-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []types.Choice{
			{Index: 0, Message: types.Message{Role: "assistant", Content: content}, FinishReason: finishReason},
		},
		Usage: usage,
	}
}
