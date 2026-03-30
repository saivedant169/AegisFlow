package eval

import (
	"bytes"
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"time"
)

// WebhookRequest is the payload sent to the external evaluation endpoint.
type WebhookRequest struct {
	RequestID    string `json:"request_id"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
	Prompt       string `json:"prompt"`
	Response     string `json:"response"`
	LatencyMs    int64  `json:"latency_ms"`
	BuiltinScore int    `json:"builtin_score"`
}

// WebhookResponse is the expected response from the evaluation endpoint.
type WebhookResponse struct {
	Score    int      `json:"score"`
	Labels   []string `json:"labels"`
	Feedback string   `json:"feedback"`
}

// WebhookEvaluator sends sampled requests to an external evaluation webhook.
type WebhookEvaluator struct {
	url             string
	sampleRate      float64
	timeout         time.Duration
	sendFullContent bool
	client          *http.Client
}

// NewWebhookEvaluator creates a WebhookEvaluator with the given configuration.
func NewWebhookEvaluator(url string, sampleRate float64, timeout time.Duration, sendFullContent bool) *WebhookEvaluator {
	if sampleRate <= 0 {
		sampleRate = 0.1
	}
	return &WebhookEvaluator{
		url:             url,
		sampleRate:      sampleRate,
		timeout:         timeout,
		sendFullContent: sendFullContent,
		client:          &http.Client{Timeout: timeout},
	}
}

// ShouldEvaluate returns true if this request should be sampled for evaluation.
func (w *WebhookEvaluator) ShouldEvaluate() bool {
	return rand.Float64() < w.sampleRate
}

// Evaluate sends the request payload to the webhook asynchronously.
// Content is truncated by default unless sendFullContent is enabled.
func (w *WebhookEvaluator) Evaluate(req WebhookRequest) {
	if !w.sendFullContent {
		req.Prompt = truncate(req.Prompt, 500)
		req.Response = truncate(req.Response, 500)
	}

	go func() {
		data, err := json.Marshal(req)
		if err != nil {
			log.Printf("eval webhook: marshal error: %v", err)
			return
		}
		resp, err := w.client.Post(w.url, "application/json", bytes.NewReader(data))
		if err != nil {
			log.Printf("eval webhook: request error: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			log.Printf("eval webhook: returned %d", resp.StatusCode)
		}
	}()
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}
