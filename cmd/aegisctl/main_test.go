package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}

func TestCmdAuditVerify(t *testing.T) {
	oldClient := client
	defer func() { client = oldClient }()

	var gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/admin/v1/audit/verify" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		gotKey = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"valid":true,"message":"hash chain intact","total_entries":12}`))
	}))
	defer server.Close()

	client = server.Client()
	out := captureStdout(t, func() {
		if err := cmdAuditVerify([]string{"--url", server.URL, "--key", "admin-key"}, defaultAdminURL); err != nil {
			t.Fatal(err)
		}
	})

	if gotKey != "admin-key" {
		t.Fatalf("expected admin key header, got %q", gotKey)
	}
	if !strings.Contains(out, "Audit integrity: VALID") || !strings.Contains(out, "Entries: 12") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCmdAuditVerifyHTTPError(t *testing.T) {
	oldClient := client
	defer func() { client = oldClient }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"insufficient permissions"}}`))
	}))
	defer server.Close()

	client = server.Client()
	if err := cmdAuditVerify([]string{"--url", server.URL}, defaultAdminURL); err == nil || !strings.Contains(err.Error(), "insufficient permissions") {
		t.Fatalf("expected permissions error, got %v", err)
	}
}
