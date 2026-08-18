package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout swaps os.Stdout for a pipe, runs fn, and returns whatever fn
// wrote to stdout. Restores the original os.Stdout in a deferred call so
// parallel tests do not interfere with each other.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		if _, err := io.Copy(&buf, r); err != nil {
			t.Errorf("io.Copy: %v", err)
		}
		close(done)
	}()

	fn()
	_ = w.Close()
	<-done
	return buf.String()
}

func TestCmdVersion_Human(t *testing.T) {
	saved := version
	version = "v0.9.0"
	defer func() { version = saved }()

	got := captureStdout(t, func() { cmdVersion(nil) })

	want := "aegisctl v0.9.0\n"
	if got != want {
		t.Fatalf("human output: got %q, want %q", got, want)
	}
	if strings.Contains(got, "\x1b") {
		t.Fatalf("human output must not contain ANSI escapes: %q", got)
	}
}

func TestCmdVersion_JSON(t *testing.T) {
	saved := version
	version = "v0.9.0"
	defer func() { version = saved }()

	got := captureStdout(t, func() { cmdVersion([]string{"--json"}) })

	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("JSON output must end with newline: %q", got)
	}
	if strings.Contains(got, "\x1b") {
		t.Fatalf("JSON output must not contain ANSI escapes: %q", got)
	}

	var out map[string]string
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("JSON output is not valid JSON: %v (got %q)", err, got)
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly one key, got %d: %v", len(out), out)
	}
	if out["version"] != "v0.9.0" {
		t.Fatalf("version field: got %q, want %q", out["version"], "v0.9.0")
	}
}

func TestCmdVersion_JSON_DevBuild(t *testing.T) {
	saved := version
	version = "dev"
	defer func() { version = saved }()

	got := captureStdout(t, func() { cmdVersion([]string{"--json"}) })

	var out map[string]string
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("JSON output is not valid JSON: %v (got %q)", err, got)
	}
	if out["version"] != "dev" {
		t.Fatalf("dev version field: got %q, want %q", out["version"], "dev")
	}
}

func TestCmdVersion_JSONShortFlag(t *testing.T) {
	saved := version
	version = "v0.9.0"
	defer func() { version = saved }()

	got := captureStdout(t, func() { cmdVersion([]string{"-json"}) })

	var out map[string]string
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("JSON output (-json) is not valid JSON: %v (got %q)", err, got)
	}
	if out["version"] != "v0.9.0" {
		t.Fatalf("version field (-json): got %q, want %q", out["version"], "v0.9.0")
	}
}
