package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientCredentialBoundary(t *testing.T) {
	received := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received <- r.Header.Get("X-API-Key") }))
	defer server.Close()
	t.Setenv("AEGISFLOW_ADMIN_URL", server.URL)
	t.Setenv("AEGISFLOW_GATEWAY_URL", server.URL)
	t.Setenv("AEGISFLOW_API_KEY", "configured-key")
	for _, test := range []struct{ path, want string }{
		{"/admin/v1/approvals", "configured-key"}, {"/v1/models", "configured-key"},
		{"/health", ""}, {"/outside", ""}, {"/admin/v10/approvals", ""},
	} {
		t.Run(test.path, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, server.URL+test.path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-API-Key", "explicit-key")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if got := <-received; got != test.want {
				t.Fatalf("credential = %q, want %q", got, test.want)
			}
			if req.Header.Get("X-API-Key") != "explicit-key" {
				t.Fatal("caller request mutated")
			}
		})
	}
}

func TestClientErrorsHideRemoteData(t *testing.T) {
	secret := "fixture-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, secret)
	}))
	defer server.Close()
	_, err := client.Get(server.URL + "?token=" + secret)
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe or missing error: %v", err)
	}
	_, err = client.Get("http://%" + secret)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe parse error: %v", err)
	}
}

func TestClientRejectsCredentialInConfiguredURL(t *testing.T) {
	t.Setenv("AEGISFLOW_ADMIN_URL", "http://user:fixture-secret@localhost")
	_, err := client.Get("http://localhost/health")
	if err == nil || !strings.Contains(err.Error(), "AEGISFLOW_ADMIN_URL") || strings.Contains(err.Error(), "fixture-secret") {
		t.Fatalf("unsafe configuration error: %v", err)
	}
}

func TestDecodeRejectsHTTPFailureBeforeJSON(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(`{"valid":true}`))}
	var result VerifyResponse
	if err := decodeJSON(resp, &result); err == nil {
		t.Fatal("HTTP failure accepted as successful payload")
	}
}
