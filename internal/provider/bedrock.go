package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

// BedrockProvider handles AWS Bedrock's Converse API.
// URL: https://bedrock-runtime.{region}.amazonaws.com/model/{modelId}/converse
// Auth: AWS Signature V4

type BedrockProvider struct {
	name      string
	region    string
	accessKey string
	secretKey string
	models    []string
	client    *http.Client
}

type bedrockRequest struct {
	Messages []bedrockMessage `json:"messages"`
	InferenceConfig *bedrockInferenceConfig `json:"inferenceConfig,omitempty"`
}

type bedrockMessage struct {
	Role    string          `json:"role"`
	Content []bedrockContent `json:"content"`
}

type bedrockContent struct {
	Text string `json:"text,omitempty"`
}

type bedrockInferenceConfig struct {
	MaxTokens   *int     `json:"maxTokens,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP        *float64 `json:"topP,omitempty"`
}

type bedrockResponse struct {
	Output struct {
		Message bedrockMessage `json:"message"`
	} `json:"output"`
	StopReason string `json:"stopReason"`
	Usage      struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
		TotalTokens  int `json:"totalTokens"`
	} `json:"usage"`
}

func NewBedrockProvider(name, region, accessKeyEnv, secretKeyEnv string, models []string, timeout time.Duration) *BedrockProvider {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	if region == "" {
		region = "us-east-1"
	}
	return &BedrockProvider{
		name:      name,
		region:    region,
		accessKey: os.Getenv(accessKeyEnv),
		secretKey: os.Getenv(secretKeyEnv),
		models:    models,
		client:    &http.Client{Timeout: timeout},
	}
}

func (b *BedrockProvider) Name() string { return b.name }

func (b *BedrockProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	brReq := b.translateRequest(req)
	body, err := json.Marshal(brReq)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com/model/%s/converse", b.region, req.Model)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	b.signRequest(httpReq, body)

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var brResp bedrockResponse
	if err := json.NewDecoder(resp.Body).Decode(&brResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return b.translateResponse(&brResp, req.Model), nil
}

func (b *BedrockProvider) ChatCompletionStream(ctx context.Context, req *types.ChatCompletionRequest) (io.ReadCloser, error) {
	// Bedrock streaming uses a different endpoint and binary event stream format.
	// For MVP, fall back to non-streaming and wrap in SSE format.
	resp, err := b.ChatCompletion(ctx, req)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		id := fmt.Sprintf("aegis-bedrock-%d", time.Now().UnixNano())

		chunk := types.StreamChunk{
			ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: req.Model,
			Choices: []types.StreamDelta{{Index: 0, Delta: types.Delta{Content: resp.Choices[0].Message.Content}}},
		}
		data, _ := json.Marshal(chunk)
		fmt.Fprintf(pw, "data: %s\n\n", data)

		finalChunk := types.StreamChunk{
			ID: id, Object: "chat.completion.chunk", Created: time.Now().Unix(), Model: req.Model,
			Choices: []types.StreamDelta{{Index: 0, Delta: types.Delta{}, FinishReason: "stop"}},
		}
		data, _ = json.Marshal(finalChunk)
		fmt.Fprintf(pw, "data: %s\n\n", data)
		fmt.Fprint(pw, "data: [DONE]\n\n")
	}()

	return pr, nil
}

func (b *BedrockProvider) Models(_ context.Context) ([]types.Model, error) {
	models := make([]types.Model, len(b.models))
	for i, m := range b.models {
		models[i] = types.Model{ID: m, Object: "model", Provider: b.name}
	}
	return models, nil
}

func (b *BedrockProvider) EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}

func (b *BedrockProvider) Healthy(_ context.Context) bool {
	return b.accessKey != "" && b.secretKey != ""
}

func (b *BedrockProvider) translateRequest(req *types.ChatCompletionRequest) bedrockRequest {
	var messages []bedrockMessage
	for _, m := range req.Messages {
		if m.Role == "system" {
			continue // Bedrock handles system prompts differently
		}
		messages = append(messages, bedrockMessage{
			Role:    m.Role,
			Content: []bedrockContent{{Text: m.Content}},
		})
	}

	brReq := bedrockRequest{Messages: messages}
	if req.Temperature != nil || req.MaxTokens != nil || req.TopP != nil {
		brReq.InferenceConfig = &bedrockInferenceConfig{
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
			TopP:        req.TopP,
		}
	}
	return brReq
}

func (b *BedrockProvider) translateResponse(resp *bedrockResponse, model string) *types.ChatCompletionResponse {
	content := ""
	for _, c := range resp.Output.Message.Content {
		content += c.Text
	}

	return &types.ChatCompletionResponse{
		ID:      fmt.Sprintf("aegis-bedrock-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []types.Choice{
			{Index: 0, Message: types.Message{Role: "assistant", Content: content}, FinishReason: "stop"},
		},
		Usage: types.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
}

// signRequest adds AWS Signature V4 headers (simplified for Bedrock).
func (b *BedrockProvider) signRequest(req *http.Request, payload []byte) {
	now := time.Now().UTC()
	datestamp := now.Format("20060102")
	amzdate := now.Format("20060102T150405Z")

	req.Header.Set("X-Amz-Date", amzdate)

	// Hash payload
	payloadHash := sha256Hex(payload)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// Canonical request
	canonicalHeaders := fmt.Sprintf("content-type:%s\nhost:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		req.Header.Get("Content-Type"), req.Host, payloadHash, amzdate)
	signedHeaders := "content-type;host;x-amz-content-sha256;x-amz-date"

	canonicalRequest := strings.Join([]string{
		req.Method, req.URL.Path, req.URL.RawQuery,
		canonicalHeaders, signedHeaders, payloadHash,
	}, "\n")

	// String to sign
	scope := fmt.Sprintf("%s/%s/bedrock/aws4_request", datestamp, b.region)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s", amzdate, scope, sha256Hex([]byte(canonicalRequest)))

	// Signing key
	kDate := hmacSHA256([]byte("AWS4"+b.secretKey), []byte(datestamp))
	kRegion := hmacSHA256(kDate, []byte(b.region))
	kService := hmacSHA256(kRegion, []byte("bedrock"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))

	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		b.accessKey, scope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
