package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCmdStatus_Healthy(t *testing.T) {
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/admin/v1/system/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"active_credentials": 2, "latest_audit_timestamp": "2026-01-01T00:00:00Z", "loaded_policy_pack": "default", "mcp_gateway": "reachable"}`))
		if err != nil {
			return
		}
	})
	adminMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "providers") || strings.Contains(r.URL.Path, "sessions") || strings.Contains(r.URL.Path, "violations") {
			_, err := w.Write([]byte(`[]`))
			if err != nil {
				return
			}
		} else {
			_, err := w.Write([]byte(`{}`))
			if err != nil {
				return
			}
		}
	})

	gwMux := http.NewServeMux()
	gwMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"status":"ok"}`))
		if err != nil {
			return
		}
	})

	adminSrv := httptest.NewServer(adminMux)
	defer adminSrv.Close()
	gwSrv := httptest.NewServer(gwMux)
	defer gwSrv.Close()

	oldStdout := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut

	emitStatusJSON(gwSrv.URL, adminSrv.URL, true, true)

	err := wOut.Close()
	if err != nil {
		return
	}
	os.Stdout = oldStdout

	outBytes, _ := io.ReadAll(rOut)
	out := string(outBytes)

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v, output: %s", err, out)
	}

	if healthy, ok := result["healthy"].(bool); !ok || !healthy {
		t.Errorf("Expected healthy to be true, got %v", result["healthy"])
	}

	sys, ok := result["system"].(map[string]any)
	if !ok {
		t.Fatalf("Expected system object in output")
	}
	if sys["mcp_gateway"] != "reachable" {
		t.Errorf("Expected mcp_gateway to be reachable, got %v", sys["mcp_gateway"])
	}
}

func TestCmdStatus_Unhealthy(t *testing.T) {
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/admin/v1/system/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"active_credentials": 2, "latest_audit_timestamp": "2026-01-01T00:00:00Z", "loaded_policy_pack": "default", "mcp_gateway": "unreachable"}`))
		if err != nil {
			return
		}
	})
	adminMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "providers") || strings.Contains(r.URL.Path, "sessions") || strings.Contains(r.URL.Path, "violations") {
			_, err := w.Write([]byte(`[]`))
			if err != nil {
				return
			}
		} else {
			_, err := w.Write([]byte(`{}`))
			if err != nil {
				return
			}
		}
	})

	gwMux := http.NewServeMux()
	gwMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(`{"status":"ok"}`))
		if err != nil {
			return
		}
	})

	adminSrv := httptest.NewServer(adminMux)
	defer adminSrv.Close()
	gwSrv := httptest.NewServer(gwMux)
	defer gwSrv.Close()

	oldStdout := os.Stdout
	rOut, wOut, _ := os.Pipe()
	os.Stdout = wOut

	emitStatusJSON(gwSrv.URL, adminSrv.URL, true, true)

	err := wOut.Close()
	if err != nil {
		return
	}
	os.Stdout = oldStdout

	outBytes, _ := io.ReadAll(rOut)
	out := string(outBytes)

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v, output: %s", err, out)
	}

	if healthy, ok := result["healthy"].(bool); !ok || healthy {
		t.Errorf("Expected healthy to be false, got %v", result["healthy"])
	}

	sys, ok := result["system"].(map[string]any)
	if !ok {
		t.Fatalf("Expected system object in output")
	}
	if sys["mcp_gateway"] != "unreachable" {
		t.Errorf("Expected mcp_gateway to be unreachable, got %v", sys["mcp_gateway"])
	}
}
