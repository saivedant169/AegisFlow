package sandbox

import "testing"

func TestHTTPSandbox_AllowedHosts(t *testing.T) {
	s := &HTTPSandbox{
		AllowedHosts: []string{"api.stripe.com", "*.slack.com"},
	}

	tests := []struct {
		url  string
		want bool
	}{
		{"https://api.stripe.com/v1/charges", false},
		{"https://hooks.slack.com/services/x", false},
		{"https://api.slack.com/chat.postMessage", false},
		{"https://evil.com/steal", true},
		{"https://example.com/api", true},
	}

	for _, tt := range tests {
		v := s.Validate("GET", tt.url, 0)
		got := v != nil
		if got != tt.want {
			t.Errorf("AllowedHosts Validate(GET, %q): got violation=%v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestHTTPSandbox_BlockedHosts(t *testing.T) {
	s := &HTTPSandbox{
		BlockedHosts: []string{"*.internal.corp", "metadata.google.internal"},
	}

	tests := []struct {
		url  string
		want bool
	}{
		{"https://api.internal.corp/secrets", true},
		{"http://metadata.google.internal/computeMetadata", true},
		{"https://api.stripe.com/v1/charges", false},
	}

	for _, tt := range tests {
		v := s.Validate("GET", tt.url, 0)
		got := v != nil
		if got != tt.want {
			t.Errorf("BlockedHosts Validate(GET, %q): got violation=%v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestHTTPSandbox_BlockedHostTakesPrecedence(t *testing.T) {
	s := &HTTPSandbox{
		AllowedHosts: []string{"*.example.com"},
		BlockedHosts: []string{"evil.example.com"},
	}

	v := s.Validate("GET", "https://evil.example.com/api", 0)
	if v == nil {
		t.Fatal("expected violation for blocked host even when in allowed pattern")
	}
	if v.Rule != "blocked_host" {
		t.Errorf("got rule=%q, want blocked_host", v.Rule)
	}
}

func TestHTTPSandbox_AllowedMethods(t *testing.T) {
	s := &HTTPSandbox{
		AllowedMethods: []string{"GET", "POST"},
	}

	tests := []struct {
		method string
		want   bool
	}{
		{"GET", false},
		{"POST", false},
		{"DELETE", true},
		{"PUT", true},
		{"PATCH", true},
	}

	for _, tt := range tests {
		v := s.Validate(tt.method, "https://example.com/api", 0)
		got := v != nil
		if got != tt.want {
			t.Errorf("AllowedMethods Validate(%q): got violation=%v, want %v", tt.method, got, tt.want)
		}
	}
}

func TestHTTPSandbox_RequireHTTPS(t *testing.T) {
	s := &HTTPSandbox{RequireHTTPS: true}

	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com/api", false},
		{"http://example.com/api", true},
	}

	for _, tt := range tests {
		v := s.Validate("GET", tt.url, 0)
		got := v != nil
		if got != tt.want {
			t.Errorf("RequireHTTPS Validate(%q): got violation=%v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestHTTPSandbox_MaxPayload(t *testing.T) {
	s := &HTTPSandbox{MaxPayloadBytes: 1024}

	tests := []struct {
		size int64
		want bool
	}{
		{512, false},
		{1024, false},
		{1025, true},
		{0, false},
	}

	for _, tt := range tests {
		v := s.Validate("POST", "https://example.com/api", tt.size)
		got := v != nil
		if got != tt.want {
			t.Errorf("MaxPayload size=%d: got violation=%v, want %v", tt.size, got, tt.want)
		}
	}
}

func TestHTTPSandbox_BlockedPaths(t *testing.T) {
	s := &HTTPSandbox{
		BlockedPaths: []string{"/admin", "/internal"},
	}

	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com/admin/users", true},
		{"https://example.com/internal/config", true},
		{"https://example.com/api/users", false},
	}

	for _, tt := range tests {
		v := s.Validate("GET", tt.url, 0)
		got := v != nil
		if got != tt.want {
			t.Errorf("BlockedPaths Validate(%q): got violation=%v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestHTTPSandbox_EmptySandbox(t *testing.T) {
	s := &HTTPSandbox{}

	v := s.Validate("DELETE", "http://evil.com/destroy", 999999)
	if v != nil {
		t.Errorf("empty sandbox should allow everything, got: %v", v)
	}
}
