package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
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

func TestCmdFederationStatus(t *testing.T) {
	oldClient := client
	defer func() { client = oldClient }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/admin/v1/federation/planes" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"us-east","healthy":true,"last_seen":"2026-04-02T17:00:00Z","requests":42}]`))
	}))
	defer server.Close()

	client = server.Client()
	out := captureStdout(t, func() {
		if err := cmdFederationStatus([]string{"--url", server.URL}, defaultAdminURL); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(out, "us-east") || !strings.Contains(out, "healthy") || !strings.Contains(out, "42") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestCmdFederationStatusEmpty(t *testing.T) {
	oldClient := client
	defer func() { client = oldClient }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client = server.Client()
	out := captureStdout(t, func() {
		if err := cmdFederationStatus([]string{"--url", server.URL}, defaultAdminURL); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(out, "NAME") {
		t.Fatalf("expected table header, got %s", out)
	}
}

func TestCmdFederationStatusFormatsZeroTime(t *testing.T) {
	oldClient := client
	defer func() { client = oldClient }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"name": "plane-a", "healthy": false, "last_seen": time.Time{}, "requests": 0,
		}})
	}))
	defer server.Close()

	client = server.Client()
	out := captureStdout(t, func() {
		if err := cmdFederationStatus([]string{"--url", server.URL}, defaultAdminURL); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "--") {
		t.Fatalf("expected zero time placeholder, got %s", out)
	}
}
