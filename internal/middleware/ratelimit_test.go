package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockLimiter struct {
	allowed bool
	err     error
}

func (m *mockLimiter) Allow(key string, cost int) (bool, error) {
	return m.allowed, m.err
}

func TestRateLimitHealthSkip(t *testing.T) {
	lim := &mockLimiter{allowed: false}
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	RateLimit(lim)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}

func TestRateLimitNoTenant(t *testing.T) {
	lim := &mockLimiter{allowed: false}
	r := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	RateLimit(lim)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}

func TestRateLimitAllowed(t *testing.T) {
	lim := &mockLimiter{allowed: true}
	r := requestWithTenant("/v1/chat/completions", "t1")
	w := httptest.NewRecorder()

	RateLimit(lim)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}

func TestRateLimitDenied(t *testing.T) {
	lim := &mockLimiter{allowed: false}
	r := requestWithTenant("/v1/chat/completions", "t1")
	w := httptest.NewRecorder()

	RateLimit(lim)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusTooManyRequests)
	if got := w.Header().Get("Retry-After"); got != "60" {
		t.Errorf("expected Retry-After 60, got %s", got)
	}
}

func TestRateLimitError(t *testing.T) {
	lim := &mockLimiter{err: errors.New("redis down")}
	r := requestWithTenant("/v1/chat/completions", "t1")
	w := httptest.NewRecorder()

	RateLimit(lim)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusServiceUnavailable)
}
