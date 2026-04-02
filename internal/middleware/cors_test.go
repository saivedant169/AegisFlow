package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/config"
)

func corsConfig(enabled bool, origins []string, creds bool) *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			CORS: config.CORSConfig{
				Enabled:          enabled,
				AllowedOrigins:   origins,
				AllowedMethods:   []string{"GET", "POST"},
				AllowedHeaders:   []string{"Content-Type", "Authorization"},
				ExposedHeaders:   []string{"X-Custom"},
				AllowCredentials: creds,
				MaxAge:           3600,
			},
		},
	}
}

func TestCORSDisabled(t *testing.T) {
	cfg := corsConfig(false, nil, false)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()

	CORS(cfg)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS headers when disabled")
	}
}

func TestCORSNoOriginHeader(t *testing.T) {
	cfg := corsConfig(true, []string{"http://example.com"}, false)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	CORS(cfg)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS headers when no Origin")
	}
}

func TestCORSAllowedOrigin(t *testing.T) {
	cfg := corsConfig(true, []string{"http://example.com"}, false)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()

	CORS(cfg)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://example.com" {
		t.Errorf("expected origin http://example.com, got %s", got)
	}
}

func TestCORSDisallowedOrigin(t *testing.T) {
	cfg := corsConfig(true, []string{"http://example.com"}, false)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()

	CORS(cfg)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("expected no CORS headers for disallowed origin")
	}
}

func TestCORSPreflightOptions(t *testing.T) {
	cfg := corsConfig(true, []string{"http://example.com"}, false)
	r := httptest.NewRequest(http.MethodOptions, "/", nil)
	r.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()

	CORS(cfg)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusNoContent)
	if got := w.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST" {
		t.Errorf("expected methods GET, POST, got %s", got)
	}
	if got := w.Header().Get("Access-Control-Max-Age"); got != "3600" {
		t.Errorf("expected max age 3600, got %s", got)
	}
}

func TestCORSCredentialsWithExplicitOrigins(t *testing.T) {
	cfg := corsConfig(true, []string{"http://example.com"}, true)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()

	CORS(cfg)(okHandler()).ServeHTTP(w, r)
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected credentials true, got %s", got)
	}
}

func TestCORSCredentialsWithWildcardBlocked(t *testing.T) {
	cfg := corsConfig(true, []string{"*"}, true)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()

	CORS(cfg)(okHandler()).ServeHTTP(w, r)
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("expected no credentials header with wildcard, got %s", got)
	}
}

func TestCORSWildcardOrigin(t *testing.T) {
	cfg := corsConfig(true, []string{"*"}, false)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://anything.com")
	w := httptest.NewRecorder()

	CORS(cfg)(okHandler()).ServeHTTP(w, r)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://anything.com" {
		t.Errorf("expected origin http://anything.com, got %s", got)
	}
}

func TestCORSExposedHeaders(t *testing.T) {
	cfg := corsConfig(true, []string{"http://example.com"}, false)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://example.com")
	w := httptest.NewRecorder()

	CORS(cfg)(okHandler()).ServeHTTP(w, r)
	if got := w.Header().Get("Access-Control-Expose-Headers"); got != "X-Custom" {
		t.Errorf("expected exposed headers X-Custom, got %s", got)
	}
}
