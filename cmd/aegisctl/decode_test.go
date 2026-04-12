package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDecodeJSONValidResponse verifies decodeJSON succeeds with valid JSON.
func TestDecodeJSONValidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	if err := decodeJSON(resp, &result); err != nil {
		t.Fatalf("decodeJSON should succeed for valid JSON, got: %v", err)
	}
	if result["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", result["status"])
	}
}

// TestDecodeJSONMalformedResponse verifies decodeJSON returns an error for
// malformed JSON instead of silently swallowing the failure.
func TestDecodeJSONMalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"broken: json`))
	}))
	defer srv.Close()

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	if err := decodeJSON(resp, &result); err == nil {
		t.Fatal("decodeJSON should return error for malformed JSON, got nil")
	}
}

// TestDecodeJSONEmptyBody verifies decodeJSON returns an error for an empty
// response body.
func TestDecodeJSONEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]string
	if err := decodeJSON(resp, &result); err == nil {
		t.Fatal("decodeJSON should return error for empty body, got nil")
	}
}

// TestFetchJSONMalformed verifies fetchJSON returns nil and includes an error
// message when the server returns invalid JSON.
func TestFetchJSONMalformed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	result := fetchJSON(srv.URL)
	if result != nil {
		t.Fatalf("fetchJSON should return nil for malformed JSON, got: %v", result)
	}
}

// TestMarshalJSONError verifies that marshalJSON returns a non-nil result and
// error for valid/invalid inputs.
func TestMarshalJSONValid(t *testing.T) {
	data, err := marshalJSON(map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("marshalJSON should succeed for valid input, got: %v", err)
	}
	if !strings.Contains(string(data), "key") {
		t.Fatalf("expected marshaled data to contain 'key', got: %s", string(data))
	}
}
