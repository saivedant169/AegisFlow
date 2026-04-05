package gateway

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

func testConfig() *config.Config {
	return &config.Config{
		Tenants: []config.TenantConfig{
			{
				ID:   "ws-test",
				Name: "WS Test Tenant",
				APIKeys: []config.APIKeyEntry{
					{Key: "ws-test-key-001", Role: "operator"},
				},
			},
		},
	}
}

func setupWSServer(t *testing.T) (*httptest.Server, *Handler, *config.Config) {
	t.Helper()
	h := setupTestHandler()
	cfg := testConfig()
	wsCfg := WebSocketConfig{Enabled: true, PingInterval: 500 * time.Millisecond}
	wsHandler := h.WebSocket(cfg, wsCfg)
	ts := httptest.NewServer(wsHandler)
	return ts, h, cfg
}

func wsURL(ts *httptest.Server, queryParams string) string {
	url := "ws" + strings.TrimPrefix(ts.URL, "http")
	if queryParams != "" {
		url += "?" + queryParams
	}
	return url
}

func wsDial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	origin := "http://localhost/"
	ws, err := websocket.Dial(url, "", origin)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	return ws
}

func wsSendJSON(t *testing.T, ws *websocket.Conn, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := websocket.Message.Send(ws, string(data)); err != nil {
		t.Fatalf("send failed: %v", err)
	}
}

func wsRecvJSON(t *testing.T, ws *websocket.Conn) wsEnvelope {
	t.Helper()
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	var raw string
	if err := websocket.Message.Receive(ws, &raw); err != nil {
		t.Fatalf("receive failed: %v", err)
	}
	var env wsEnvelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal envelope failed: %v", err)
	}
	return env
}

func TestWebSocketUpgradeSucceeds(t *testing.T) {
	ts, _, _ := setupWSServer(t)
	defer ts.Close()

	ws := wsDial(t, wsURL(ts, "api_key=ws-test-key-001"))
	defer ws.Close()

	// Connection established successfully. Send a ping and expect a pong or
	// just close gracefully.
	wsSendJSON(t, ws, wsEnvelope{Type: "ping", ID: "p1"})
	env := wsRecvJSON(t, ws)
	if env.Type != "pong" {
		t.Errorf("expected pong, got %s", env.Type)
	}
	if env.ID != "p1" {
		t.Errorf("expected id p1, got %s", env.ID)
	}
}

func TestWebSocketRequestResponse(t *testing.T) {
	ts, _, _ := setupWSServer(t)
	defer ts.Close()

	ws := wsDial(t, wsURL(ts, "api_key=ws-test-key-001"))
	defer ws.Close()

	// Send a chat completion request.
	reqPayload := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello via WebSocket"}},
	}
	payloadBytes, _ := json.Marshal(reqPayload)
	raw := json.RawMessage(payloadBytes)

	wsSendJSON(t, ws, wsEnvelope{Type: "request", ID: "req-1", Payload: &raw})

	// We may receive a ping before the response; skip pings.
	var env wsEnvelope
	for {
		env = wsRecvJSON(t, ws)
		if env.Type != "ping" {
			break
		}
	}

	if env.Type != "response" {
		t.Fatalf("expected response, got %s", env.Type)
	}
	if env.ID != "req-1" {
		t.Errorf("expected id req-1, got %s", env.ID)
	}

	var resp types.ChatCompletionResponse
	if err := json.Unmarshal(*env.Payload, &resp); err != nil {
		t.Fatalf("unmarshal response payload failed: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected at least one choice")
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason stop, got %s", resp.Choices[0].FinishReason)
	}
}

func TestWebSocketMultipleRequests(t *testing.T) {
	ts, _, _ := setupWSServer(t)
	defer ts.Close()

	ws := wsDial(t, wsURL(ts, "api_key=ws-test-key-001"))
	defer ws.Close()

	for i := 0; i < 3; i++ {
		reqPayload := types.ChatCompletionRequest{
			Model:    "mock",
			Messages: []types.Message{{Role: "user", Content: "request"}},
		}
		payloadBytes, _ := json.Marshal(reqPayload)
		raw := json.RawMessage(payloadBytes)
		wsSendJSON(t, ws, wsEnvelope{Type: "request", ID: "multi", Payload: &raw})

		var env wsEnvelope
		for {
			env = wsRecvJSON(t, ws)
			if env.Type != "ping" {
				break
			}
		}
		if env.Type != "response" {
			t.Fatalf("iteration %d: expected response, got %s", i, env.Type)
		}
	}
}

func TestWebSocketConnectionCloseHandling(t *testing.T) {
	ts, _, _ := setupWSServer(t)
	defer ts.Close()

	ws := wsDial(t, wsURL(ts, "api_key=ws-test-key-001"))

	// Close the client side. The server should handle this gracefully.
	ws.Close()

	// If the server panicked or leaked, the test would fail with a timeout
	// or panic. Simply reaching here means the close was handled.
}

func TestWebSocketInvalidMessageFormat(t *testing.T) {
	ts, _, _ := setupWSServer(t)
	defer ts.Close()

	ws := wsDial(t, wsURL(ts, "api_key=ws-test-key-001"))
	defer ws.Close()

	// Send invalid JSON.
	if err := websocket.Message.Send(ws, "not valid json{{{"); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	// Should receive an error envelope.
	var env wsEnvelope
	for {
		env = wsRecvJSON(t, ws)
		if env.Type != "ping" {
			break
		}
	}
	if env.Type != "error" {
		t.Fatalf("expected error, got %s", env.Type)
	}
}

func TestWebSocketInvalidAPIKey(t *testing.T) {
	ts, _, _ := setupWSServer(t)
	defer ts.Close()

	ws := wsDial(t, wsURL(ts, "api_key=bad-key"))
	defer ws.Close()

	// The server should send an error and close.
	env := wsRecvJSON(t, ws)
	if env.Type != "error" {
		t.Fatalf("expected error for bad key, got %s", env.Type)
	}
}

func TestWebSocketAuthViaFirstMessage(t *testing.T) {
	ts, _, _ := setupWSServer(t)
	defer ts.Close()

	// Connect without api_key query param.
	ws := wsDial(t, wsURL(ts, ""))
	defer ws.Close()

	// Send auth message.
	authPayload, _ := json.Marshal(map[string]string{"api_key": "ws-test-key-001"})
	raw := json.RawMessage(authPayload)
	wsSendJSON(t, ws, wsEnvelope{Type: "auth", ID: "a1", Payload: &raw})

	env := wsRecvJSON(t, ws)
	if env.Type != "auth_ok" {
		t.Fatalf("expected auth_ok, got %s (payload: %v)", env.Type, env.Payload)
	}

	// Now send a request.
	reqPayload := types.ChatCompletionRequest{
		Model:    "mock",
		Messages: []types.Message{{Role: "user", Content: "Hello after auth"}},
	}
	payloadBytes, _ := json.Marshal(reqPayload)
	rawReq := json.RawMessage(payloadBytes)
	wsSendJSON(t, ws, wsEnvelope{Type: "request", ID: "req-auth", Payload: &rawReq})

	for {
		env = wsRecvJSON(t, ws)
		if env.Type != "ping" {
			break
		}
	}
	if env.Type != "response" {
		t.Fatalf("expected response, got %s", env.Type)
	}
}

func TestWebSocketMissingModel(t *testing.T) {
	ts, _, _ := setupWSServer(t)
	defer ts.Close()

	ws := wsDial(t, wsURL(ts, "api_key=ws-test-key-001"))
	defer ws.Close()

	// Request with no model.
	reqPayload := types.ChatCompletionRequest{
		Messages: []types.Message{{Role: "user", Content: "Hello"}},
	}
	payloadBytes, _ := json.Marshal(reqPayload)
	raw := json.RawMessage(payloadBytes)
	wsSendJSON(t, ws, wsEnvelope{Type: "request", ID: "bad", Payload: &raw})

	var env wsEnvelope
	for {
		env = wsRecvJSON(t, ws)
		if env.Type != "ping" {
			break
		}
	}
	if env.Type != "error" {
		t.Fatalf("expected error for missing model, got %s", env.Type)
	}
}

func TestWebSocketServerPing(t *testing.T) {
	ts, _, _ := setupWSServer(t)
	defer ts.Close()

	ws := wsDial(t, wsURL(ts, "api_key=ws-test-key-001"))
	defer ws.Close()

	// Wait for server to send a ping (ping interval is 500ms in test config).
	env := wsRecvJSON(t, ws)
	if env.Type != "ping" {
		t.Fatalf("expected server ping, got %s", env.Type)
	}
}
