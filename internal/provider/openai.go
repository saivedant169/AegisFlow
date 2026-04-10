package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

type OpenAIProvider struct {
	name       string
	baseURL    string
	keys       *KeyRotator
	models     []string
	client     *http.Client
	maxRetries int
	retry      retryPolicy
	sleep      func(time.Duration)
}

// NewOpenAIProvider creates a provider with a single API key read from the
// named environment variable. Use NewOpenAIProviderWithKeys for multi-key rotation.
func NewOpenAIProvider(name, baseURL, apiKeyEnv string, models []string, timeout time.Duration, maxRetries int) *OpenAIProvider {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	if maxRetries == 0 {
		maxRetries = 2
	}
	key := os.Getenv(apiKeyEnv)
	return &OpenAIProvider{
		name:       name,
		baseURL:    baseURL,
		keys:       NewKeyRotator([]string{key}, "round-robin", 0),
		models:     models,
		client:     &http.Client{Timeout: timeout},
		maxRetries: maxRetries,
		retry:      newRetryPolicy(config.RetryConfig{MaxAttempts: 1}),
		sleep:      time.Sleep,
	}
}

// NewOpenAIProviderWithKeys creates a provider that rotates across multiple API keys.
func NewOpenAIProviderWithKeys(name, baseURL string, keys *KeyRotator, models []string, timeout time.Duration, maxRetries int) *OpenAIProvider {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	if maxRetries == 0 {
		maxRetries = 2
	}
	return &OpenAIProvider{
		name:       name,
		baseURL:    baseURL,
		keys:       keys,
		models:     models,
		client:     &http.Client{Timeout: timeout},
		maxRetries: maxRetries,
		retry:      newRetryPolicy(config.RetryConfig{MaxAttempts: 1}),
		sleep:      time.Sleep,
	}
}

func (o *OpenAIProvider) Name() string {
	return o.name
}

func (o *OpenAIProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	key, ok := o.keys.Pick()
	if !ok {
		return nil, fmt.Errorf("provider %s: no available API keys", o.name)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := o.doRequest(ctx, body, key)
	if err != nil {
		o.markKeyFromError(key, err)
		return nil, err
	}
	defer resp.Body.Close()

	var result types.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result, nil
}

func (o *OpenAIProvider) ChatCompletionStream(ctx context.Context, req *types.ChatCompletionRequest) (io.ReadCloser, error) {
	key, ok := o.keys.Pick()
	if !ok {
		return nil, fmt.Errorf("provider %s: no available API keys", o.name)
	}

	streamReq := *req
	streamReq.Stream = true

	body, err := json.Marshal(streamReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := o.doRequest(ctx, body, key)
	if err != nil {
		o.markKeyFromError(key, err)
		return nil, err
	}

	return resp.Body, nil
}

func (o *OpenAIProvider) Models(ctx context.Context) ([]types.Model, error) {
	models := make([]types.Model, len(o.models))
	for i, m := range o.models {
		models[i] = types.Model{
			ID:       m,
			Object:   "model",
			Provider: o.name,
		}
	}
	return models, nil
}

func (o *OpenAIProvider) EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}

func (o *OpenAIProvider) Healthy(ctx context.Context) bool {
	key, ok := o.keys.Pick()
	if !ok {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.baseURL+"/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := o.client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (o *OpenAIProvider) ConfigureRetry(cfg config.RetryConfig) {
	o.retry = newRetryPolicy(cfg)
}

// markKeyFromError inspects the error and marks the key as failed or rate-limited accordingly.
func (o *OpenAIProvider) markKeyFromError(key string, err error) {
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		return
	}
	switch statusErr.StatusCode {
	case http.StatusUnauthorized:
		o.keys.MarkFailed(key)
	case http.StatusTooManyRequests:
		o.keys.MarkRateLimited(key)
	}
}

// doRequest executes the HTTP POST to the provider with retry logic.
// On each retry the key is re-picked from the rotator so that a key marked
// rate-limited or failed during a previous attempt is not reused.
func (o *OpenAIProvider) doRequest(ctx context.Context, body []byte, key string) (*http.Response, error) {
	attempts := o.retry.maxAttempts
	if attempts <= 0 {
		attempts = 1
	}

	for attempt := 1; attempt <= attempts; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+key)

		resp, err := o.client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("sending request: %w", err)
		}
		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		statusErr := &HTTPStatusError{StatusCode: resp.StatusCode, Body: string(respBody), Header: resp.Header.Clone()}
		if attempt < attempts && o.retry.shouldRetry(resp.StatusCode) {
			// Mark the current key before picking a fresh one for the next attempt,
			// so a 429-limited key is not retried immediately.
			o.markKeyFromError(key, statusErr)
			if next, ok := o.keys.Pick(); ok {
				key = next
			}
			delay := o.retry.delayForAttempt(attempt, resp.Header)
			log.Printf("provider %s: retry attempt %d/%d after %s due to status %d", o.name, attempt+1, attempts, delay, resp.StatusCode)
			o.sleep(delay)
			continue
		}
		return nil, statusErr
	}

	return nil, fmt.Errorf("provider %s exhausted retries", o.name)
}
