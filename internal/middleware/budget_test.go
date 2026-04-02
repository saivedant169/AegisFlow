package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/saivedant169/AegisFlow/internal/config"
)

func requestWithTenant(path string, tenantID string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, nil)
	if tenantID != "" {
		tenant := &config.TenantConfig{ID: tenantID}
		ctx := context.WithValue(r.Context(), TenantContextKey, tenant)
		r = r.WithContext(ctx)
	}
	return r
}

func TestBudgetCheckNilFn(t *testing.T) {
	r := requestWithTenant("/v1/chat/completions", "t1")
	w := httptest.NewRecorder()

	BudgetCheck(nil)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}

func TestBudgetCheckNonCompletionsPath(t *testing.T) {
	called := false
	fn := func(tenantID, model string) (bool, []string, string) {
		called = true
		return true, nil, ""
	}
	r := requestWithTenant("/v1/models", "t1")
	w := httptest.NewRecorder()

	BudgetCheck(fn)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
	if called {
		t.Error("expected checkFn not to be called for non-completions path")
	}
}

func TestBudgetCheckHealthPath(t *testing.T) {
	called := false
	fn := func(tenantID, model string) (bool, []string, string) {
		called = true
		return true, nil, ""
	}
	r := requestWithTenant("/health", "t1")
	w := httptest.NewRecorder()

	BudgetCheck(fn)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
	if called {
		t.Error("expected checkFn not to be called for health path")
	}
}

func TestBudgetCheckAllowed(t *testing.T) {
	fn := func(tenantID, model string) (bool, []string, string) {
		return true, nil, ""
	}
	r := requestWithTenant("/v1/chat/completions", "t1")
	w := httptest.NewRecorder()

	BudgetCheck(fn)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}

func TestBudgetCheckBlocked(t *testing.T) {
	fn := func(tenantID, model string) (bool, []string, string) {
		return false, nil, "budget exceeded"
	}
	r := requestWithTenant("/v1/chat/completions", "t1")
	w := httptest.NewRecorder()

	BudgetCheck(fn)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusTooManyRequests)
}

func TestBudgetCheckWarnings(t *testing.T) {
	fn := func(tenantID, model string) (bool, []string, string) {
		return true, []string{"80% used", "90% used"}, ""
	}
	r := requestWithTenant("/v1/chat/completions", "t1")
	w := httptest.NewRecorder()

	BudgetCheck(fn)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
	warnings := w.Header().Values("X-AegisFlow-Budget-Warning")
	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings, got %d", len(warnings))
	}
}

func TestBudgetCheckNoTenant(t *testing.T) {
	fn := func(tenantID, model string) (bool, []string, string) {
		return false, nil, "should not reach"
	}
	r := requestWithTenant("/v1/chat/completions", "")
	w := httptest.NewRecorder()

	BudgetCheck(fn)(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusOK)
}
