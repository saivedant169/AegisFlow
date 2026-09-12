package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAdminTransportScopesCredentialAndRejectsRedirect(t *testing.T) {
	var leaked atomic.Bool
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "" {
			leaked.Store(true)
		}
	}))
	defer foreign.Close()
	received := make(chan string, 2)
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Get("X-API-Key")
		if r.URL.Path == "/admin/v1/redirect" {
			http.Redirect(w, r, foreign.URL, http.StatusFound)
		}
	}))
	defer admin.Close()
	t.Setenv("AEGISFLOW_API_KEY", "reviewer-key")
	t.Setenv("AEGISFLOW_ADMIN_URL", admin.URL)
	for _, route := range []string{"/admin/v1/evidence/sessions", "/admin/v1/redirect"} {
		resp, err := client.Get(admin.URL + route)
		if route == "/admin/v1/redirect" {
			if err == nil || !strings.Contains(err.Error(), "HTTP 302") {
				t.Fatalf("expected redirect rejection, got %v", err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		if <-received != "reviewer-key" {
			t.Fatal("admin credential missing")
		}
	}
	resp, err := client.Get(foreign.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if leaked.Load() {
		t.Fatal("admin credential leaked to another origin")
	}
}
