package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type Event struct {
	EventType  string `json:"event_type"`
	PolicyName string `json:"policy_name"`
	Action     string `json:"action"`
	TenantID   string `json:"tenant_id"`
	Model      string `json:"model"`
	Message    string `json:"message"`
	Timestamp  string `json:"timestamp"`
}

type Notifier struct {
	url    string
	secret string
	client *http.Client
}

func NewNotifier(url string, secret ...string) *Notifier {
	if url == "" {
		return nil
	}
	s := ""
	if len(secret) > 0 {
		s = secret[0]
	}
	return &Notifier{
		url:    url,
		secret: s,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// ComputeSignature computes HMAC-SHA256 over "timestamp.body" using the secret.
func ComputeSignature(secret string, timestamp int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", timestamp, body)))
	return hex.EncodeToString(mac.Sum(nil))
}

// Send fires a webhook asynchronously. Never blocks the request path.
func (n *Notifier) Send(event Event) {
	if n == nil {
		return
	}
	event.Timestamp = time.Now().UTC().Format(time.RFC3339)
	go func() {
		data, err := json.Marshal(event)
		if err != nil {
			log.Printf("webhook: failed to marshal event: %v", err)
			return
		}

		req, err := http.NewRequest(http.MethodPost, n.url, bytes.NewReader(data))
		if err != nil {
			log.Printf("webhook: failed to create request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		if n.secret != "" {
			ts := time.Now().Unix()
			sig := ComputeSignature(n.secret, ts, data)
			req.Header.Set("X-AegisFlow-Signature", sig)
			req.Header.Set("X-AegisFlow-Timestamp", fmt.Sprintf("%d", ts))
		}

		resp, err := n.client.Do(req)
		if err != nil {
			log.Printf("webhook: failed to send to %s: %v", n.url, err)
			return
		}
		resp.Body.Close()

		if resp.StatusCode >= 400 {
			log.Printf("webhook: %s returned status %d", n.url, resp.StatusCode)
		}
	}()
}
