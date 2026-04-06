package sandbox

import (
	"testing"
	"time"
)

func TestShellSandbox_BlockedBinary(t *testing.T) {
	s := &ShellSandbox{
		BlockedBinaries: []string{"rm", "dd", "mkfs"},
	}

	tests := []struct {
		cmd  string
		want bool
	}{
		{"rm", true},
		{"dd", true},
		{"mkfs", true},
		{"ls", false},
		{"cat", false},
		{"/bin/rm", true}, // absolute path should also match
	}

	for _, tt := range tests {
		v := s.Validate(tt.cmd, nil, "/tmp")
		got := v != nil
		if got != tt.want {
			t.Errorf("Validate(%q): got violation=%v, want %v", tt.cmd, got, tt.want)
		}
		if v != nil && v.Rule != "blocked_binary" {
			t.Errorf("Validate(%q): got rule=%q, want blocked_binary", tt.cmd, v.Rule)
		}
	}
}

func TestShellSandbox_AllowedBinary(t *testing.T) {
	s := &ShellSandbox{
		AllowedBinaries: []string{"ls", "cat", "grep"},
	}

	tests := []struct {
		cmd  string
		want bool
	}{
		{"ls", false},
		{"cat", false},
		{"grep", false},
		{"rm", true},
		{"curl", true},
	}

	for _, tt := range tests {
		v := s.Validate(tt.cmd, nil, "/tmp")
		got := v != nil
		if got != tt.want {
			t.Errorf("Validate(%q): got violation=%v, want %v", tt.cmd, got, tt.want)
		}
	}
}

func TestShellSandbox_BlockedBinaryTakesPrecedence(t *testing.T) {
	s := &ShellSandbox{
		AllowedBinaries: []string{"rm", "ls"},
		BlockedBinaries: []string{"rm"},
	}

	v := s.Validate("rm", nil, "/tmp")
	if v == nil {
		t.Fatal("expected violation for rm (blocked takes precedence over allowed)")
	}
	if v.Rule != "blocked_binary" {
		t.Errorf("got rule=%q, want blocked_binary", v.Rule)
	}
}

func TestShellSandbox_BlockedPaths(t *testing.T) {
	s := &ShellSandbox{
		BlockedPaths: []string{"/etc/shadow", "/root", "/home/user/.env"},
	}

	tests := []struct {
		cmd     string
		args    []string
		workDir string
		want    bool
	}{
		{"cat", []string{"/etc/shadow"}, "/tmp", true},
		{"ls", []string{"/root/secrets"}, "/tmp", true},
		{"cat", []string{"/home/user/.env"}, "/tmp", true},
		{"ls", nil, "/tmp", false},
		{"cat", []string{"/var/log/syslog"}, "/tmp", false},
		{"ls", nil, "/root", true}, // workDir in blocked path
	}

	for _, tt := range tests {
		v := s.Validate(tt.cmd, tt.args, tt.workDir)
		got := v != nil
		if got != tt.want {
			t.Errorf("Validate(%q, %v, %q): got violation=%v, want %v", tt.cmd, tt.args, tt.workDir, got, tt.want)
		}
	}
}

func TestShellSandbox_AllowedPaths(t *testing.T) {
	s := &ShellSandbox{
		AllowedPaths: []string{"/home/agent/workspace", "/tmp"},
	}

	tests := []struct {
		workDir string
		want    bool
	}{
		{"/home/agent/workspace", false},
		{"/home/agent/workspace/src", false},
		{"/tmp", false},
		{"/etc", true},
		{"/root", true},
	}

	for _, tt := range tests {
		v := s.Validate("ls", nil, tt.workDir)
		got := v != nil
		if got != tt.want {
			t.Errorf("Validate with workDir=%q: got violation=%v, want %v", tt.workDir, got, tt.want)
		}
	}
}

func TestShellSandbox_EnvRedaction(t *testing.T) {
	s := &ShellSandbox{
		EnvRedactions: []string{"AWS_SECRET_*", "GITHUB_TOKEN", "DATABASE_PASSWORD"},
	}

	tests := []struct {
		envName string
		want    bool
	}{
		{"AWS_SECRET_ACCESS_KEY", true},
		{"AWS_SECRET_KEY", true},
		{"GITHUB_TOKEN", true},
		{"DATABASE_PASSWORD", true},
		{"HOME", false},
		{"PATH", false},
		{"AWS_REGION", false},
	}

	for _, tt := range tests {
		got := s.ShouldRedactEnv(tt.envName)
		if got != tt.want {
			t.Errorf("ShouldRedactEnv(%q): got %v, want %v", tt.envName, got, tt.want)
		}
	}
}

func TestShellSandbox_EmptySandbox(t *testing.T) {
	s := &ShellSandbox{}

	// An empty sandbox should allow everything.
	v := s.Validate("rm", []string{"-rf", "/"}, "/")
	if v != nil {
		t.Errorf("empty sandbox should allow everything, got: %v", v)
	}
}

func TestShellSandbox_FieldsPresent(t *testing.T) {
	s := &ShellSandbox{
		MaxDuration:    30 * time.Second,
		MaxOutputBytes: 1024 * 1024,
		NetworkPolicy: NetworkPolicy{
			AllowEgress:  true,
			AllowedHosts: []string{"api.example.com"},
			BlockedHosts: []string{"evil.com"},
			BlockedPorts: []int{22, 3389},
		},
	}

	if s.MaxDuration != 30*time.Second {
		t.Errorf("MaxDuration: got %v, want 30s", s.MaxDuration)
	}
	if s.MaxOutputBytes != 1024*1024 {
		t.Errorf("MaxOutputBytes: got %v, want 1048576", s.MaxOutputBytes)
	}
	if !s.NetworkPolicy.AllowEgress {
		t.Error("NetworkPolicy.AllowEgress should be true")
	}
}
