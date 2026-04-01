package eval

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewWebhookEvaluatorNegativeSampleRate(t *testing.T) {
	we := NewWebhookEvaluator("http://example.com", -1, 5*time.Second, false)
	if we.sampleRate != 0.1 {
		t.Errorf("expected default sample rate 0.1 for negative input, got %f", we.sampleRate)
	}
}

func TestNewWebhookEvaluatorZeroSampleRate(t *testing.T) {
	we := NewWebhookEvaluator("http://example.com", 0, 5*time.Second, false)
	if we.sampleRate != 0.1 {
		t.Errorf("expected default sample rate 0.1 for zero input, got %f", we.sampleRate)
	}
}

func TestNewWebhookEvaluatorValidSampleRate(t *testing.T) {
	we := NewWebhookEvaluator("http://example.com", 0.5, 5*time.Second, true)
	if we.sampleRate != 0.5 {
		t.Errorf("expected sample rate 0.5, got %f", we.sampleRate)
	}
	if !we.sendFullContent {
		t.Error("expected sendFullContent to be true")
	}
}

func TestShouldEvaluateRate0(t *testing.T) {
	// sampleRate <= 0 defaults to 0.1, so we test with a very small positive value
	// that makes ShouldEvaluate nearly always false, but to truly test rate=0 behavior,
	// we construct manually.
	we := &WebhookEvaluator{sampleRate: 0}
	// With sampleRate 0, rand.Float64() is always >= 0, so ShouldEvaluate should be false
	for i := 0; i < 100; i++ {
		if we.ShouldEvaluate() {
			t.Fatal("ShouldEvaluate should always return false when sampleRate is 0")
		}
	}
}

func TestShouldEvaluateRate1(t *testing.T) {
	we := &WebhookEvaluator{sampleRate: 1.0}
	// With sampleRate 1.0, rand.Float64() is always < 1.0, so ShouldEvaluate should be true
	for i := 0; i < 100; i++ {
		if !we.ShouldEvaluate() {
			t.Fatal("ShouldEvaluate should always return true when sampleRate is 1.0")
		}
	}
}

func TestTruncateShorterThanMax(t *testing.T) {
	s := "hello"
	result := truncate(s, 100)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncateExactLength(t *testing.T) {
	s := "hello"
	result := truncate(s, 5)
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestTruncateLongerThanMax(t *testing.T) {
	s := "hello world"
	result := truncate(s, 5)
	if result != "hello..." {
		t.Errorf("expected 'hello...', got %q", result)
	}
}

func TestTruncateUnicodeMultibyte(t *testing.T) {
	// Each emoji is one rune but multiple bytes
	s := "Hello \U0001F600\U0001F601\U0001F602\U0001F603"
	result := truncate(s, 8)
	// Should be "Hello " (6 runes) + 2 emojis = 8 runes, then "..."
	expected := "Hello \U0001F600\U0001F601..."
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestTruncateEmptyString(t *testing.T) {
	result := truncate("", 10)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestTruncateUnicodeJapanese(t *testing.T) {
	s := "日本語テスト"
	result := truncate(s, 3)
	if result != "日本語..." {
		t.Errorf("expected '日本語...', got %q", result)
	}
}

func TestEvaluateSendsRequest(t *testing.T) {
	received := make(chan WebhookRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req WebhookRequest
		json.NewDecoder(r.Body).Decode(&req)
		received <- req
		json.NewEncoder(w).Encode(WebhookResponse{Score: 90, Labels: []string{"good"}})
	}))
	defer server.Close()

	we := NewWebhookEvaluator(server.URL, 1.0, 5*time.Second, true)
	we.Evaluate(WebhookRequest{
		RequestID:    "test-1",
		Model:        "gpt-4",
		Provider:     "openai",
		Prompt:       "hello",
		Response:     "world",
		LatencyMs:    100,
		BuiltinScore: 95,
	})

	select {
	case req := <-received:
		if req.Model != "gpt-4" {
			t.Errorf("expected model gpt-4, got %s", req.Model)
		}
		if req.Prompt != "hello" {
			t.Errorf("expected full prompt 'hello', got %q", req.Prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook request")
	}
}

func TestEvaluateTruncatesContent(t *testing.T) {
	received := make(chan WebhookRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req WebhookRequest
		json.NewDecoder(r.Body).Decode(&req)
		received <- req
		w.WriteHeader(200)
	}))
	defer server.Close()

	we := NewWebhookEvaluator(server.URL, 1.0, 5*time.Second, false) // sendFullContent=false

	longPrompt := ""
	for i := 0; i < 600; i++ {
		longPrompt += "a"
	}

	we.Evaluate(WebhookRequest{
		Prompt:   longPrompt,
		Response: longPrompt,
	})

	select {
	case req := <-received:
		// truncate at 500 runes + "..."
		if len(req.Prompt) != 503 {
			t.Errorf("expected truncated prompt length 503, got %d", len(req.Prompt))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for webhook request")
	}
}

func TestEvaluateHandlesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer server.Close()

	we := NewWebhookEvaluator(server.URL, 1.0, 5*time.Second, false)
	// Should not panic on server error
	we.Evaluate(WebhookRequest{Prompt: "test", Response: "test"})
	time.Sleep(500 * time.Millisecond) // give goroutine time to complete
}
