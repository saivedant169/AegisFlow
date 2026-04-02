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

// AzureOpenAIProvider handles Azure OpenAI Service which uses a different
// URL scheme and auth header than standard OpenAI.
// URL format: {endpoint}/openai/deployments/{deployment}/chat/completions?api-version={version}
// Auth: api-key header instead of Authorization: Bearer

type AzureOpenAIProvider struct {
	name       string
	endpoint   string // e.g. https://myresource.openai.azure.com
	apiKey     string
	apiVersion string
	models     []string // model names map to deployment names
	client     *http.Client
}

func NewAzureOpenAIProvider(name, endpoint, apiKeyEnv, apiVersion string, models []string, timeout time.Duration) *AzureOpenAIProvider {
	apiKey := os.Getenv(apiKeyEnv)
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	if apiVersion == "" {
		apiVersion = "2024-10-21"
	}
	return &AzureOpenAIProvider{
		name:       name,
		endpoint:   endpoint,
		apiKey:     apiKey,
		apiVersion: apiVersion,
		models:     models,
		client:     &http.Client{Timeout: timeout},
	}
}

func (a *AzureOpenAIProvider) Name() string {
	return a.name
}

func (a *AzureOpenAIProvider) buildURL(deployment string) string {
	return fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", a.endpoint, deployment, a.apiVersion)
}

func (a *AzureOpenAIProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := a.buildURL(req.Model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", a.apiKey)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result types.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

func (a *AzureOpenAIProvider) ChatCompletionStream(ctx context.Context, req *types.ChatCompletionRequest) (io.ReadCloser, error) {
	streamReq := *req
	streamReq.Stream = true

	body, err := json.Marshal(streamReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := a.buildURL(req.Model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", a.apiKey)

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

func (a *AzureOpenAIProvider) Models(_ context.Context) ([]types.Model, error) {
	models := make([]types.Model, len(a.models))
	for i, m := range a.models {
		models[i] = types.Model{ID: m, Object: "model", Provider: a.name}
	}
	return models, nil
}

func (a *AzureOpenAIProvider) EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}

func (a *AzureOpenAIProvider) Healthy(ctx context.Context) bool {
	return a.apiKey != ""
}
