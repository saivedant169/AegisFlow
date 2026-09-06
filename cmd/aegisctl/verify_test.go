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
)

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()
	<-done
	return buf.String()
}

func TestCmdVerifyJSONUsesSelectedEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantPath string
	}{
		{name: "audit", args: []string{"--json"}, wantPath: "/admin/v1/audit/verify"},
		{name: "session", args: []string{"--session", "session-42", "--json"}, wantPath: "/admin/v1/evidence/sessions/session-42/verify"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"valid":true,"total_records":4,"message":"evidence chain signatures verified"}`))
			}))
			defer srv.Close()

			got := captureStdout(t, func() { cmdVerify(srv.URL, tt.args) })
			if gotPath != tt.wantPath {
				t.Fatalf("request path = %q, want %q", gotPath, tt.wantPath)
			}
			if strings.Contains(got, "\x1b") {
				t.Fatalf("JSON output contains ANSI escapes: %q", got)
			}
			var result VerifyResponse
			if err := json.Unmarshal([]byte(got), &result); err != nil {
				t.Fatalf("output is not JSON: %v (got %q)", err, got)
			}
			if !result.Valid || result.TotalRecords != 4 || result.Message != "evidence chain signatures verified" {
				t.Fatalf("unexpected JSON result: %+v", result)
			}
		})
	}
}

func TestCmdVerifyJSONInvalidChainReturnsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"valid":false,"total_records":4,"error_at_index":2,"message":"signature mismatch"}`))
	}))
	defer srv.Close()

	code := 0
	got := captureStdout(t, func() { code = cmdVerify(srv.URL, []string{"--json"}) })
	if code != 1 {
		t.Fatalf("exit status = %d, want 1", code)
	}
	var result VerifyResponse
	if err := json.Unmarshal([]byte(got), &result); err != nil {
		t.Fatalf("output is not JSON: %v (got %q)", err, got)
	}
	if result.Valid || result.ErrorAtIndex != 2 {
		t.Fatalf("unexpected invalid-chain result: %+v", result)
	}
}

func TestCmdVerifyErrorsReturnFailureOnStderr(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantStderr string
	}{
		{
			name: "HTTP status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
			},
			wantStderr: "Error (503)",
		},
		{
			name: "decode",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`not-json`))
			},
			wantStderr: "Error parsing response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			code := 0
			got := captureStderr(t, func() { code = cmdVerify(srv.URL, []string{"--json"}) })
			if code != 1 {
				t.Fatalf("exit status = %d, want 1", code)
			}
			if !strings.Contains(got, tt.wantStderr) {
				t.Fatalf("stderr = %q, want substring %q", got, tt.wantStderr)
			}
		})
	}
}

func TestCmdVerifyHumanOutputIsPreserved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"valid":true,"total_records":4,"message":"verified"}`))
	}))
	defer srv.Close()

	code := 1
	got := captureStdout(t, func() { code = cmdVerify(srv.URL, nil) })
	if code != 0 {
		t.Fatalf("exit status = %d, want 0", code)
	}
	if !strings.Contains(got, "\x1b[32mPASS\x1b[0m") || !strings.Contains(got, "Total entries: 4") {
		t.Fatalf("unexpected human output: %q", got)
	}
}

func TestFormatVerifyResult_Pass(t *testing.T) {
	r := VerifyResponse{
		Valid:        true,
		TotalRecords: 42,
		Message:      "evidence chain integrity verified",
	}
	out := formatVerifyResult(r)
	if !strings.Contains(out, "PASS") {
		t.Fatal("expected PASS in output")
	}
	if !strings.Contains(out, "Total entries: 42") {
		t.Fatal("expected total entries in output")
	}
	if strings.Contains(out, "Error at index") {
		t.Fatal("should not show error index for valid result")
	}
}

func TestFormatVerifyResult_Fail(t *testing.T) {
	r := VerifyResponse{
		Valid:        false,
		TotalRecords: 10,
		ErrorAtIndex: 5,
		Message:      "hash mismatch at record abc123",
	}
	out := formatVerifyResult(r)
	if !strings.Contains(out, "FAIL") {
		t.Fatal("expected FAIL in output")
	}
	if !strings.Contains(out, "Error at index: 5") {
		t.Fatal("expected error index in output")
	}
	if !strings.Contains(out, "hash mismatch") {
		t.Fatal("expected message in output")
	}
}

func TestFormatVerifyResult_EmptyChain(t *testing.T) {
	r := VerifyResponse{
		Valid:        true,
		TotalRecords: 0,
		Message:      "empty chain is valid",
	}
	out := formatVerifyResult(r)
	if !strings.Contains(out, "PASS") {
		t.Fatal("expected PASS for empty chain")
	}
	if !strings.Contains(out, "Total entries: 0") {
		t.Fatal("expected zero entries")
	}
}
