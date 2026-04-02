package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Tenants: []config.TenantConfig{
			{ID: "t1", APIKeys: []config.APIKeyEntry{{Key: "valid-key", Role: "admin"}}},
		},
	}
}

func TestAuthValidKeyXAPIKey(t *testing.T) {
	cfg := testConfig()
	r := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	r.Header.Set("X-API-Key", "valid-key")
	w := httptest.NewRecorder()

	Auth(cfg)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}

func TestAuthValidKeyBearer(t *testing.T) {
	cfg := testConfig()
	r := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	r.Header.Set("Authorization", "Bearer valid-key")
	w := httptest.NewRecorder()

	Auth(cfg)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}

func TestAuthInvalidKey(t *testing.T) {
	cfg := testConfig()
	r := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	r.Header.Set("X-API-Key", "bad-key")
	w := httptest.NewRecorder()

	Auth(cfg)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestAuthMissingKey(t *testing.T) {
	cfg := testConfig()
	r := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	Auth(cfg)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusUnauthorized)
}

func TestAuthHealthPathSkip(t *testing.T) {
	cfg := testConfig()
	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	Auth(cfg)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}

func TestAuthSetsTenantAndRoleInContext(t *testing.T) {
	cfg := testConfig()
	r := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	r.Header.Set("X-API-Key", "valid-key")
	w := httptest.NewRecorder()

	var gotTenantID, gotRole string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := TenantFromContext(r.Context())
		if tenant != nil {
			gotTenantID = tenant.ID
		}
		gotRole = RoleFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	Auth(cfg)(handler).ServeHTTP(w, r)

	if gotTenantID != "t1" {
		t.Errorf("expected tenant t1, got %s", gotTenantID)
	}
	if gotRole != "admin" {
		t.Errorf("expected role admin, got %s", gotRole)
	}
}

// SoftAuth tests

func TestSoftAuthValidKey(t *testing.T) {
	cfg := testConfig()
	r := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	r.Header.Set("X-API-Key", "valid-key")
	w := httptest.NewRecorder()

	var gotTenantID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := TenantFromContext(r.Context())
		if tenant != nil {
			gotTenantID = tenant.ID
		}
		w.WriteHeader(http.StatusOK)
	})

	SoftAuth(cfg)(handler).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
	if gotTenantID != "t1" {
		t.Errorf("expected tenant t1, got %s", gotTenantID)
	}
}

func TestSoftAuthInvalidKey(t *testing.T) {
	cfg := testConfig()
	r := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	r.Header.Set("X-API-Key", "bad-key")
	w := httptest.NewRecorder()

	SoftAuth(cfg)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK) // passes through
}

func TestSoftAuthMissingKey(t *testing.T) {
	cfg := testConfig()
	r := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	SoftAuth(cfg)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK) // passes through
}

// Context helper tests

func TestTenantFromContextNil(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	tenant := TenantFromContext(r.Context())
	if tenant != nil {
		t.Errorf("expected nil tenant, got %v", tenant)
	}
}

func TestRoleFromContextEmpty(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	role := RoleFromContext(r.Context())
	if role != "" {
		t.Errorf("expected empty role, got %s", role)
	}
}
