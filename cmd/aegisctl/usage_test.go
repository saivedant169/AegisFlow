package main
import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)
func TestCmdUsage_JSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/v1/usage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"tenant-a": {
				"by_model": {
					"gpt-4": {"requests": 3, "total_tokens": 120, "estimated_cost_usd": 0.045}
				}
			}
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	got := captureStdout(t, func() { cmdUsage(srv.URL, true) })
	if strings.Contains(got, "\x1b") {
		t.Fatalf("JSON output must not contain ANSI escapes: %q", got)
	}
	var out struct {
		Tenants []struct {
			Tenant string `json:"tenant"`
			Models []struct {
				Model            string  `json:"model"`
				Requests         float64 `json:"requests"`
				TotalTokens      float64 `json:"total_tokens"`
				EstimatedCostUSD float64 `json:"estimated_cost_usd"`
			} `json:"models"`
		} `json:"tenants"`
	}
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("JSON output is not valid JSON: %v (got %q)", err, got)
	}
	if len(out.Tenants) != 1 {
		t.Fatalf("expected exactly one tenant, got %d: %+v", len(out.Tenants), out.Tenants)
	}
	tenant := out.Tenants[0]
	if tenant.Tenant != "tenant-a" {
		t.Fatalf("tenant field: got %q, want %q", tenant.Tenant, "tenant-a")
	}
	if len(tenant.Models) != 1 {
		t.Fatalf("expected exactly one model, got %d: %+v", len(tenant.Models), tenant.Models)
	}
	model := tenant.Models[0]
	if model.Model != "gpt-4" {
		t.Fatalf("model field: got %q, want %q", model.Model, "gpt-4")
	}
	if model.Requests != 3 {
		t.Fatalf("requests: got %v, want 3", model.Requests)
	}
	if model.TotalTokens != 120 {
		t.Fatalf("total_tokens: got %v, want 120", model.TotalTokens)
	}
	if model.EstimatedCostUSD != 0.045 {
		t.Fatalf("estimated_cost_usd: got %v, want 0.045", model.EstimatedCostUSD)
	}
}
func TestCmdUsage_JSON_Empty(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/v1/usage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	got := captureStdout(t, func() { cmdUsage(srv.URL, true) })
	var out struct {
		Tenants []interface{} `json:"tenants"`
	}
	if err := json.Unmarshal([]byte(got), &out); err != nil {
		t.Fatalf("JSON output is not valid JSON: %v (got %q)", err, got)
	}
	if len(out.Tenants) != 0 {
		t.Fatalf("expected empty tenants list, got %+v", out.Tenants)
	}
}
func TestCmdUsage_Human(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/admin/v1/usage", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"tenant-a": {
				"by_model": {
					"gpt-4": {"requests": 3, "total_tokens": 120, "estimated_cost_usd": 0.045}
				}
			}
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	got := captureStdout(t, func() { cmdUsage(srv.URL, false) })
	if !strings.Contains(got, "TENANT") || !strings.Contains(got, "tenant-a") || !strings.Contains(got, "gpt-4") {
		t.Fatalf("expected human table output, got %q", got)
	}
	if strings.Contains(got, "{") {
		t.Fatalf("human output should not contain JSON, got %q", got)
	}
}