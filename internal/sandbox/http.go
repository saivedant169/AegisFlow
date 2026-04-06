package sandbox

import (
	"net/url"
	"strings"
)

// HTTPSandbox enforces constraints on outbound HTTP requests.
type HTTPSandbox struct {
	AllowedHosts    []string `yaml:"allowed_hosts" json:"allowed_hosts"`
	BlockedHosts    []string `yaml:"blocked_hosts" json:"blocked_hosts"`
	AllowedMethods  []string `yaml:"allowed_methods" json:"allowed_methods"`
	MaxPayloadBytes int64    `yaml:"max_payload_bytes" json:"max_payload_bytes"`
	BlockedPaths    []string `yaml:"blocked_paths" json:"blocked_paths"`
	RequireHTTPS    bool     `yaml:"require_https" json:"require_https"`
}

// Validate checks an HTTP request against sandbox constraints and returns the
// first violation found, or nil if the request is allowed.
func (s *HTTPSandbox) Validate(method, rawURL string, bodySize int64) *SandboxViolation {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return &SandboxViolation{
			SandboxType: "http",
			Rule:        "invalid_url",
			Message:     "cannot parse URL: " + rawURL,
			Severity:    "block",
		}
	}

	// Require HTTPS.
	if s.RequireHTTPS && parsed.Scheme == "http" {
		return &SandboxViolation{
			SandboxType: "http",
			Rule:        "require_https",
			Message:     "plain HTTP is blocked; use HTTPS",
			Severity:    "block",
		}
	}

	host := strings.ToLower(parsed.Hostname())

	// Check blocked hosts (always takes precedence).
	for _, blocked := range s.BlockedHosts {
		if matchHostPattern(host, strings.ToLower(blocked)) {
			return &SandboxViolation{
				SandboxType: "http",
				Rule:        "blocked_host",
				Message:     "host is explicitly blocked: " + host,
				Severity:    "block",
			}
		}
	}

	// If an allowlist is configured, the host must be in it.
	if len(s.AllowedHosts) > 0 {
		allowed := false
		for _, a := range s.AllowedHosts {
			if matchHostPattern(host, strings.ToLower(a)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return &SandboxViolation{
				SandboxType: "http",
				Rule:        "allowed_host",
				Message:     "host not in allowlist: " + host,
				Severity:    "block",
			}
		}
	}

	// Check allowed methods.
	if len(s.AllowedMethods) > 0 {
		methodUpper := strings.ToUpper(method)
		allowed := false
		for _, m := range s.AllowedMethods {
			if strings.ToUpper(m) == methodUpper {
				allowed = true
				break
			}
		}
		if !allowed {
			return &SandboxViolation{
				SandboxType: "http",
				Rule:        "allowed_method",
				Message:     "HTTP method not allowed: " + method,
				Severity:    "block",
			}
		}
	}

	// Check blocked paths.
	requestPath := parsed.Path
	for _, blocked := range s.BlockedPaths {
		if matchGlob(requestPath, blocked) || strings.HasPrefix(requestPath, blocked) {
			return &SandboxViolation{
				SandboxType: "http",
				Rule:        "blocked_path",
				Message:     "path is blocked: " + requestPath,
				Severity:    "block",
			}
		}
	}

	// Check payload size.
	if s.MaxPayloadBytes > 0 && bodySize > s.MaxPayloadBytes {
		return &SandboxViolation{
			SandboxType: "http",
			Rule:        "max_payload",
			Message:     "request body exceeds maximum allowed size",
			Severity:    "block",
		}
	}

	return nil
}

// matchHostPattern checks if a host matches a pattern, supporting wildcard
// subdomains (e.g. "*.example.com" matches "api.example.com").
func matchHostPattern(host, pattern string) bool {
	if host == pattern {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		return strings.HasSuffix(host, suffix)
	}
	return false
}
