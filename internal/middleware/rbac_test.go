package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func requestWithRole(role string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if role != "" {
		ctx := context.WithValue(r.Context(), RoleContextKey, role)
		r = r.WithContext(ctx)
	}
	return r
}

func assertStatus(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("got status %d, want %d", got, want)
	}
}

func TestRBACAdminAllowedEverywhere(t *testing.T) {
	w := httptest.NewRecorder()
	RBAC("admin")(okHandler()).ServeHTTP(w, requestWithRole("admin"))
	assertStatus(t, w.Code, http.StatusOK)
}

func TestRBACOperatorBlockedFromAdmin(t *testing.T) {
	w := httptest.NewRecorder()
	RBAC("admin")(okHandler()).ServeHTTP(w, requestWithRole("operator"))
	assertStatus(t, w.Code, http.StatusForbidden)
}

func TestRBACViewerBlockedFromOperator(t *testing.T) {
	w := httptest.NewRecorder()
	RBAC("operator")(okHandler()).ServeHTTP(w, requestWithRole("viewer"))
	assertStatus(t, w.Code, http.StatusForbidden)
}

func TestRBACViewerAllowedOnViewer(t *testing.T) {
	w := httptest.NewRecorder()
	RBAC("viewer")(okHandler()).ServeHTTP(w, requestWithRole("viewer"))
	assertStatus(t, w.Code, http.StatusOK)
}

func TestRBACOperatorAllowedOnViewer(t *testing.T) {
	w := httptest.NewRecorder()
	RBAC("viewer")(okHandler()).ServeHTTP(w, requestWithRole("operator"))
	assertStatus(t, w.Code, http.StatusOK)
}

func TestRBACAdminAllowedOnOperator(t *testing.T) {
	w := httptest.NewRecorder()
	RBAC("operator")(okHandler()).ServeHTTP(w, requestWithRole("admin"))
	assertStatus(t, w.Code, http.StatusOK)
}

func TestRBACNoRoleReturns403(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	RBAC("viewer")(okHandler()).ServeHTTP(w, r)
	assertStatus(t, w.Code, http.StatusForbidden)
}

func TestRBACUnknownRoleReturns403(t *testing.T) {
	w := httptest.NewRecorder()
	RBAC("viewer")(okHandler()).ServeHTTP(w, requestWithRole("unknown"))
	assertStatus(t, w.Code, http.StatusForbidden)
}
