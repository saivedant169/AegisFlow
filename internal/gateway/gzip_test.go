package gateway

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestGzipDecodingValid(t *testing.T) {
	h := setupTestHandler() // from handler_test.go
	h.maxBodySize = 1024 * 1024

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)

	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, err := w.Write(body)
	if err != nil {
		return
	}
	err = w.Close()
	if err != nil {
		return
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", &b)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	rw := httptest.NewRecorder()

	h.ChatCompletion(rw, req)

	if rw.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rw.Code)
	}
}

func TestGzipDecodingInvalid(t *testing.T) {
	h := setupTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte("not a gzip")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	rw := httptest.NewRecorder()

	h.ChatCompletion(rw, req)

	if rw.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid gzip, got %d", rw.Code)
	}
}

func TestGzipDecodingZipBomb(t *testing.T) {
	h := setupTestHandler()
	h.maxBodySize = 100 // strict limit for test

	// Create a payload that compresses to very small but decompresses to > 100 bytes
	largeString := strings.Repeat("a", 200)
	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: largeString}},
	}
	body, _ := json.Marshal(reqBody)

	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, err := w.Write(body)
	if err != nil {
		return
	}
	err = w.Close()
	if err != nil {
		return
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", &b)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	rw := httptest.NewRecorder()

	h.ChatCompletion(rw, req)

	if rw.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for zip bomb exceeding maxBodySize, got %d", rw.Code)
	}
	respStr := rw.Body.String()
	if !strings.Contains(respStr, "request body too large") {
		t.Errorf("expected body too large error, got %s", respStr)
	}
}

func TestGzipEncoding(t *testing.T) {
	h := setupTestHandler()
	h.SetCompression(true, 50) // enable, threshold 50 bytes

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello world this is a test"}}, // should produce > 50 bytes response
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	rw := httptest.NewRecorder()

	h.ChatCompletion(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rw.Code)
	}

	if rw.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("expected Content-Encoding: gzip, got %q", rw.Header().Get("Content-Encoding"))
	}
	if rw.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("expected Vary: Accept-Encoding, got %q", rw.Header().Get("Vary"))
	}

	gr, err := gzip.NewReader(rw.Body)
	if err != nil {
		t.Fatalf("failed to read gzip response: %v", err)
	}
	defer func(gr *gzip.Reader) {
		err := gr.Close()
		if err != nil {
			t.Fatalf("failed to close gzip reader: %v", err)
		}
	}(gr)
	decompressed, _ := io.ReadAll(gr)

	var resp types.ChatCompletionResponse
	if err := json.Unmarshal(decompressed, &resp); err != nil {
		t.Fatalf("failed to unmarshal decompressed response: %v", err)
	}
	if len(resp.Choices) != 1 {
		t.Errorf("expected 1 choice, got %d", len(resp.Choices))
	}
}

func TestGzipEncodingDisabled(t *testing.T) {
	h := setupTestHandler()
	h.SetCompression(false, 10) // disabled

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	rw := httptest.NewRecorder()

	h.ChatCompletion(rw, req)

	if rw.Header().Get("Content-Encoding") == "gzip" {
		t.Error("expected no gzip encoding when disabled")
	}
}

func TestGzipEncodingBelowThreshold(t *testing.T) {
	h := setupTestHandler()
	h.SetCompression(true, 10000) // high threshold

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	rw := httptest.NewRecorder()

	h.ChatCompletion(rw, req)

	if rw.Header().Get("Content-Encoding") == "gzip" {
		t.Error("expected no gzip encoding when response is below threshold")
	}
}

func TestGzipEncodingStream(t *testing.T) {
	h := setupTestHandler()
	h.SetCompression(true, 10)

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
		Stream:   true,
	}
	body, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	rw := httptest.NewRecorder()

	h.ChatCompletion(rw, req)

	if rw.Header().Get("Content-Encoding") == "gzip" {
		t.Error("expected no gzip encoding for streams")
	}
}

func BenchmarkGzipEncoding(b *testing.B) {
	h := setupTestHandler()
	h.SetCompression(true, 50)

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello benchmark testing"}},
	}
	body, _ := json.Marshal(reqBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept-Encoding", "gzip")
		rw := httptest.NewRecorder()
		h.ChatCompletion(rw, req)
	}
}

func BenchmarkGzipDecoding(b *testing.B) {
	h := setupTestHandler()
	h.maxBodySize = 10 * 1024 * 1024

	reqBody := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello benchmark testing"}},
	}
	body, _ := json.Marshal(reqBody)
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(body)
	if err != nil {
		return
	}
	err = w.Close()
	if err != nil {
		return
	}
	gzBody := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(gzBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Encoding", "gzip")
		rw := httptest.NewRecorder()
		h.ChatCompletion(rw, req)
	}
}
