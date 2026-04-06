package httpgate

import (
	"fmt"
	"strings"
)

// ServiceConfig describes an upstream HTTP service that the proxy can route to.
type ServiceConfig struct {
	Name        string `yaml:"name" json:"name"`
	UpstreamURL string `yaml:"upstream_url" json:"upstream_url"`
	PathPrefix  string `yaml:"path_prefix" json:"path_prefix"`
}

// MatchService finds the first service whose PathPrefix matches the request path.
// Returns nil if no service matches.
func MatchService(host string, path string, services []ServiceConfig) *ServiceConfig {
	for i := range services {
		prefix := services[i].PathPrefix
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(path, prefix) {
			return &services[i]
		}
	}
	return nil
}

// BuildToolName creates a readable tool name from service name, HTTP method, and path.
// Format: "{service}.{method}_{first_meaningful_path_segment}"
// Example: "stripe.post_charges", "slack.post_messages"
func BuildToolName(service string, method string, path string) string {
	method = strings.ToLower(method)

	// Strip leading slashes and the service prefix path, then take the first segment.
	cleaned := strings.TrimLeft(path, "/")
	parts := strings.Split(cleaned, "/")

	// Find the first non-empty segment that isn't the service name itself.
	segment := "root"
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || strings.EqualFold(p, service) {
			continue
		}
		segment = p
		break
	}

	return fmt.Sprintf("%s.%s_%s", service, method, segment)
}
