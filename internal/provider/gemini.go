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
	Contents         []geminiContent         `json:"contents"`
	Tools            []geminiTool            `json:"tools,omitempty"`
	ToolConfig       *geminiToolConfig       `json:"toolConfig,omitempty"`
	GenerationConfig *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

// geminiFunctionCall is a model's tool call (args is a JSON object).
type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// geminiFunctionResponse returns a tool result, keyed by function name (Gemini
// has no call ids) with an object response.
type geminiFunctionResponse struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response,omitempty"`
}

// geminiTool wraps a batch of function declarations (Gemini's native shape).
type geminiTool struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiToolConfig struct {
	FunctionCallingConfig *geminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type geminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"` // AUTO | ANY | NONE
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature *float64 `json:"temperature,omitempty"`
	MaxTokens   *int     `json:"maxOutputTokens,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsage      `json:"usageMetadata"`
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
		client:  httpx.Client(timeout),
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
	// Gemini's functionResponse keys by function name, but a tool-result message
	// references the call by id, so map ids to names from the assistant turns.
	idToName := map[string]string{}
	for _, m := range req.Messages {
		for _, tc := range m.ToolCalls {
			if tc.ID != "" {
				idToName[tc.ID] = tc.Function.Name
			}
		}
	}

	var contents []geminiContent
	for _, m := range req.Messages {
		role := m.Role
		switch role {
		case "assistant":
			role = "model"
		case "system":
			role = "user" // Gemini has no system role; fold into user context
		}

		switch {
		case m.Role == "tool":
			contents = append(contents, geminiContent{Role: "user", Parts: []geminiPart{{
				FunctionResponse: &geminiFunctionResponse{Name: idToName[m.ToolCallID], Response: wrapToolResponse(m.Content)},
			}}})
		case len(m.ToolCalls) > 0:
			parts := make([]geminiPart, 0, 1+len(m.ToolCalls))
			if m.Content != "" {
				parts = append(parts, geminiPart{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				args := json.RawMessage(tc.Function.Arguments)
				if len(args) == 0 {
					args = json.RawMessage("{}")
				}
				parts = append(parts, geminiPart{FunctionCall: &geminiFunctionCall{Name: tc.Function.Name, Args: args}})
			}
			contents = append(contents, geminiContent{Role: role, Parts: parts})
		default:
			contents = append(contents, geminiContent{Role: role, Parts: []geminiPart{{Text: m.Content}}})
		}
	}

	gemReq := geminiRequest{Contents: contents}

	if len(req.Tools) > 0 {
		decls := make([]geminiFunctionDecl, 0, len(req.Tools))
		for _, t := range req.Tools {
			decls = append(decls, geminiFunctionDecl{Name: t.Function.Name, Description: t.Function.Description, Parameters: t.Function.Parameters})
		}
		gemReq.Tools = []geminiTool{{FunctionDeclarations: decls}}
		gemReq.ToolConfig = geminiToolConfigFrom(req.ToolChoice)
	}

	if req.Temperature != nil || req.MaxTokens != nil || req.TopP != nil {
		gemReq.GenerationConfig = &geminiGenerationConfig{
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
			TopP:        req.TopP,
		}
	}

	return gemReq
}

// wrapToolResponse returns the tool output as a JSON object, which Gemini's
// functionResponse requires. A JSON-object string passes through; anything else
// is wrapped as {"result": <content>}.
func wrapToolResponse(content string) json.RawMessage {
	var v map[string]interface{}
	if json.Unmarshal([]byte(content), &v) == nil {
		return json.RawMessage(content)
	}
	b, _ := json.Marshal(map[string]string{"result": content})
	return b
}

// geminiToolConfigFrom maps the internal (OpenAI-form) tool_choice to Gemini's
// functionCallingConfig: auto->AUTO, required->ANY, none->NONE, named->ANY with
// an allowed-function restriction.
func geminiToolConfigFrom(raw json.RawMessage) *geminiToolConfig {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		mode := ""
		switch s {
		case "auto":
			mode = "AUTO"
		case "required":
			mode = "ANY"
		case "none":
			mode = "NONE"
		}
		if mode == "" {
			return nil
		}
		return &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: mode}}
	}
	var obj struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Function.Name != "" {
		return &geminiToolConfig{FunctionCallingConfig: &geminiFunctionCallingConfig{Mode: "ANY", AllowedFunctionNames: []string{obj.Function.Name}}}
	}
	return nil
}

func (g *GeminiProvider) translateResponse(resp *geminiResponse, model string) *types.ChatCompletionResponse {
	content := ""
	finishReason := "stop"
	var toolCalls []types.ToolCall
	if len(resp.Candidates) > 0 {
		for _, part := range resp.Candidates[0].Content.Parts {
			if part.FunctionCall != nil {
				args := string(part.FunctionCall.Args)
				if args == "" {
					args = "{}"
				}
				toolCalls = append(toolCalls, types.ToolCall{
					Type:     "function",
					Function: types.ToolCallFunction{Name: part.FunctionCall.Name, Arguments: args},
				})
				continue
			}
			content += part.Text
		}
		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
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
			{Index: 0, Message: types.Message{Role: "assistant", Content: content, ToolCalls: toolCalls}, FinishReason: finishReason},
		},
		Usage: usage,
	}
}
