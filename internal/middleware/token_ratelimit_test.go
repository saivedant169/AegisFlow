package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/config"
)

func TestTokenRateLimitHealthSkip(t *testing.T) {
	lim := &mockLimiter{allowed: false}
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	TokenRateLimit(lim)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}

func TestTokenRateLimitNonCompletionsPath(t *testing.T) {
	lim := &mockLimiter{allowed: false}
	r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()

	TokenRateLimit(lim)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}

func TestTokenRateLimitNoTenant(t *testing.T) {
	lim := &mockLimiter{allowed: true}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4"}`))
	w := httptest.NewRecorder()

	TokenRateLimit(lim)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}

func TestTokenRateLimitAllowed(t *testing.T) {
	lim := &mockLimiter{allowed: true}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4"}`))
	r = addTenantContext(r, "t1")
	w := httptest.NewRecorder()

	TokenRateLimit(lim)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}

func TestTokenRateLimitDenied(t *testing.T) {
	lim := &mockLimiter{allowed: false}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4"}`))
	r = addTenantContext(r, "t1")
	w := httptest.NewRecorder()

	TokenRateLimit(lim)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusTooManyRequests)
}

func TestTokenRateLimitError(t *testing.T) {
	lim := &mockLimiter{err: errors.New("redis down")}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4"}`))
	r = addTenantContext(r, "t1")
	w := httptest.NewRecorder()

	TokenRateLimit(lim)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusServiceUnavailable)
}

func addTenantContext(r *http.Request, tenantID string) *http.Request {
	tenant := &config.TenantConfig{ID: tenantID}
	ctx := context.WithValue(r.Context(), TenantContextKey, tenant)
	return r.WithContext(ctx)
}
