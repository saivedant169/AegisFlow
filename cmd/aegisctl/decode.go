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
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("empty response body (HTTP %d)", resp.StatusCode)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("decoding JSON response: %w", err)
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
