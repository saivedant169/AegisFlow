package httpgate

import (
	"testing"
)

func TestMatchServiceByPathPrefix(t *testing.T) {
	services := []ServiceConfig{
		{Name: "stripe", UpstreamURL: "https://api.stripe.com", PathPrefix: "/stripe"},
		{Name: "slack", UpstreamURL: "https://slack.com/api", PathPrefix: "/slack"},
	}

	tests := []struct {
		name     string
		path     string
		wantName string
	}{
		{"matches stripe", "/stripe/v1/charges", "stripe"},
		{"matches slack", "/slack/chat.postMessage", "slack"},
		{"matches stripe root", "/stripe", "stripe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := MatchService("localhost", tt.path, services)
			if svc == nil {
				t.Fatalf("expected service %s, got nil", tt.wantName)
			}
			if svc.Name != tt.wantName {
				t.Errorf("expected service %s, got %s", tt.wantName, svc.Name)
			}
		})
	}
}

func TestMatchServiceNoMatch(t *testing.T) {
	services := []ServiceConfig{
		{Name: "stripe", UpstreamURL: "https://api.stripe.com", PathPrefix: "/stripe"},
	}

	svc := MatchService("localhost", "/github/repos", services)
	if svc != nil {
		t.Errorf("expected nil, got service %s", svc.Name)
	}
}

func TestBuildToolName(t *testing.T) {
	tests := []struct {
		name    string
		service string
		method  string
		path    string
		want    string
	}{
		{
			name:    "stripe post charges",
			service: "stripe",
			method:  "POST",
			path:    "/stripe/v1/charges",
			want:    "stripe.post_v1",
		},
		{
			name:    "slack post messages",
			service: "slack",
			method:  "POST",
			path:    "/slack/chat.postMessage",
			want:    "slack.post_chat.postMessage",
		},
		{
			name:    "get request",
			service: "stripe",
			method:  "GET",
			path:    "/stripe/v1/customers",
			want:    "stripe.get_v1",
		},
		{
			name:    "delete request",
			service: "stripe",
			method:  "DELETE",
			path:    "/stripe/v1/customers/cus_123",
			want:    "stripe.delete_v1",
		},
		{
			name:    "root path",
			service: "stripe",
			method:  "GET",
			path:    "/stripe",
			want:    "stripe.get_root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildToolName(tt.service, tt.method, tt.path)
			if got != tt.want {
				t.Errorf("BuildToolName(%q, %q, %q) = %q, want %q", tt.service, tt.method, tt.path, got, tt.want)
			}
		})
	}
}
