package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// decodeJSON reads the response body and decodes it into dst.
// Returns a descriptive error if reading or unmarshaling fails.
func decodeJSON(resp *http.Response, dst interface{}) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil {
		return fmt.Errorf("could not read API response")
	}
	if len(data) > 16<<20 {
		return fmt.Errorf("API response exceeds 16 MiB")
	}
	if len(data) == 0 {
		return fmt.Errorf("empty response body (HTTP %d)", resp.StatusCode)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("invalid JSON response")
	}
	return nil
}

// marshalJSON is a checked wrapper around json.Marshal.
func marshalJSON(v interface{}) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding JSON: %w", err)
	}
	return data, nil
}
