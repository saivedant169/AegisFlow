package webhook

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Event struct {
	EventType string `json:"event_type"`
	PolicyName string `json:"policy_name"`
	Action     string `json:"action"`
	TenantID   string `json:"tenant_id"`
	Model      string `json:"model"`
	Message    string `json:"message"`
	Timestamp  string `json:"timestamp"`
}

type Notifier struct {
	url    string
	client *http.Client
}

func NewNotifier(url string) *Notifier {
	if url == "" {
		return nil
	}
	return &Notifier{
		url: url,
		client: &http.Client{Timeout: 5 * time.Second},
	}
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

		resp, err := n.client.Post(n.url, "application/json", bytes.NewReader(data))
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
