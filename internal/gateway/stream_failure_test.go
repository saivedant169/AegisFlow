package gateway

import (
	"context"
	"errors"
	"fmt"
	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/internal/router"
	"github.com/saivedant169/AegisFlow/pkg/types"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type countedStream struct {
	reads  int
	closed bool
}

func (s *countedStream) Read(b []byte) (int, error) {
	s.reads++
	if s.reads > 4 {
		return 0, errors.New("upstream should have been stopped")
	}
	payload := `data: {"choices":[{"delta":{"content":"` + strings.Repeat("safe ", 1000) + `"}}]}` + "\n\n"
	return copy(b, payload), nil
}
func (s *countedStream) Close() error { s.closed = true; return nil }

type disconnectProvider struct {
	toolEchoProvider
	stream *countedStream
}

func (p disconnectProvider) ChatCompletionStream(context.Context, *types.ChatCompletionRequest) (io.ReadCloser, error) {
	return p.stream, nil
}

type disconnectedWriter struct {
	headers   http.Header
	attempts  int
	failAfter int
}

func (w *disconnectedWriter) Header() http.Header { return w.headers }
func (w *disconnectedWriter) WriteHeader(int)     {}
func (w *disconnectedWriter) Flush()              {}
func (w *disconnectedWriter) Write(b []byte) (int, error) {
	w.attempts++
	if w.attempts <= w.failAfter {
		return len(b), nil
	}
	return 0, io.ErrClosedPipe
}

func TestStreamDisconnectStopsUpstream(t *testing.T) {
	for _, path := range []string{"/v1/chat/completions", "/v1/messages"} {
		for _, inspect := range []bool{true, false} {
			t.Run(path+fmt.Sprint(inspect), func(t *testing.T) {
				h := setupTestHandler()
				stream := &countedStream{}
				h.registry.Register(disconnectProvider{stream: stream})
				h.router = router.NewRouter([]config.RouteConfig{{Match: config.RouteMatch{Model: "*"}, Providers: []string{"toolecho"}, Strategy: "priority"}}, h.registry)
				if !inspect {
					h.policy = nil
				}
				req := httptest.NewRequest("POST", path, strings.NewReader(`{"model":"mock","stream":true,"max_tokens":10,"messages":[{"role":"user","content":"hello"}]}`))
				w := &disconnectedWriter{headers: make(http.Header)}
				if path == "/v1/messages" {
					h.Messages(w, req)
				} else {
					h.ChatCompletion(w, req)
				}
				if !stream.closed || stream.reads > 1 || w.attempts != 1 {
					t.Fatalf("closed=%v reads=%d writes=%d", stream.closed, stream.reads, w.attempts)
				}
			})
		}
	}
}

func TestMessagesDeltaDisconnectStopsUpstream(t *testing.T) {
	for _, inspect := range []bool{true, false} {
		h := setupTestHandler()
		stream := &countedStream{}
		h.registry.Register(disconnectProvider{stream: stream})
		h.router = router.NewRouter([]config.RouteConfig{{Match: config.RouteMatch{Model: "*"}, Providers: []string{"toolecho"}, Strategy: "priority"}}, h.registry)
		if !inspect {
			h.policy = nil
		}
		req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"mock","stream":true,"max_tokens":10,"messages":[{"role":"user","content":"hello"}]}`))
		// Two startup events succeed; the first delta fails.
		w := &disconnectedWriter{headers: make(http.Header), failAfter: 8}
		h.Messages(w, req)
		if !stream.closed || stream.reads != 1 || w.attempts != 9 {
			t.Fatalf("inspect=%v closed=%v reads=%d writes=%d", inspect, stream.closed, stream.reads, w.attempts)
		}
	}
}
