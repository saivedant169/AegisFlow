package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCmdProviders_JSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/v1/providers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"openai","type":"openai","enabled":true,"healthy":false,"models":["gpt-4o"]}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got := captureStdout(t, func() { cmdProviders(srv.URL, true) })
	var out struct {
		Providers []struct {
			Name   string   `json:"name"`
			Type   string   `json:"type"`
			Status string   `json:"status"`
			Health string   `json:"health"`
			Models []string `json:"models"`
		} `json:"providers"`
	}
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("JSON output is not valid JSON: %v (got %q)", err, got)
	}
	if len(out.Providers) != 1 {
		t.Fatalf("expected one provider, got %d: %+v", len(out.Providers), out.Providers)
	}
	provider := out.Providers[0]
	if provider.Name != "openai" || provider.Type != "openai" {
		t.Fatalf("unexpected provider identity: %+v", provider)
	}
	if provider.Status != "enabled" || provider.Health != "unhealthy" {
		t.Fatalf("unexpected provider state: %+v", provider)
	}
	if len(provider.Models) != 1 || provider.Models[0] != "gpt-4o" {
		t.Fatalf("unexpected provider models: %+v", provider.Models)
	}
}

func TestCmdProviders_Human(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/v1/providers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"openai","type":"openai","enabled":true,"healthy":false,"models":["gpt-4o"]}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got := captureStdout(t, func() { cmdProviders(srv.URL, false) })
	want := "NAME    TYPE    STATUS   HEALTH     MODELS\n" +
		"────    ────    ──────   ──────     ──────\n" +
		"openai  openai  enabled  unhealthy  gpt-4o\n"
	if got != want {
		t.Fatalf("human output changed: got %q, want %q", got, want)
	}
}
