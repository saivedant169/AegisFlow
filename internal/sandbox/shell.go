package sandbox

import (
	"path/filepath"
	"strings"
	"time"
)

// NetworkPolicy controls egress network access from shell commands.
type NetworkPolicy struct {
	AllowEgress  bool     `yaml:"allow_egress" json:"allow_egress"`
	AllowedHosts []string `yaml:"allowed_hosts" json:"allowed_hosts"`
	BlockedHosts []string `yaml:"blocked_hosts" json:"blocked_hosts"`
	BlockedPorts []int    `yaml:"blocked_ports" json:"blocked_ports"`
}

// ShellSandbox enforces constraints on shell command execution.
type ShellSandbox struct {
	AllowedBinaries []string      `yaml:"allowed_binaries" json:"allowed_binaries"`
	BlockedBinaries []string      `yaml:"blocked_binaries" json:"blocked_binaries"`
	AllowedPaths    []string      `yaml:"allowed_paths" json:"allowed_paths"`
	BlockedPaths    []string      `yaml:"blocked_paths" json:"blocked_paths"`
	NetworkPolicy   NetworkPolicy `yaml:"network_policy" json:"network_policy"`
	MaxDuration     time.Duration `yaml:"max_duration" json:"max_duration"`
	MaxOutputBytes  int64         `yaml:"max_output_bytes" json:"max_output_bytes"`
	EnvRedactions   []string      `yaml:"env_redactions" json:"env_redactions"`
}

// Validate checks a shell command against sandbox constraints and returns the
// first violation found, or nil if the command is allowed.
func (s *ShellSandbox) Validate(cmd string, args []string, workDir string) *SandboxViolation {
	binary := baseCommand(cmd)

	// Check blocked binaries first (always takes precedence).
	for _, blocked := range s.BlockedBinaries {
		if matchGlob(binary, blocked) {
			return &SandboxViolation{
				SandboxType: "shell",
				Rule:        "blocked_binary",
				Message:     "binary is explicitly blocked: " + binary,
				Severity:    "block",
			}
		}
	}

	// If an allowlist is configured, the binary must be in it.
	if len(s.AllowedBinaries) > 0 {
		allowed := false
		for _, a := range s.AllowedBinaries {
			if matchGlob(binary, a) {
				allowed = true
				break
			}
		}
		if !allowed {
			return &SandboxViolation{
				SandboxType: "shell",
				Rule:        "allowed_binary",
				Message:     "binary not in allowlist: " + binary,
				Severity:    "block",
			}
		}
	}

	// Check blocked paths against working directory and arguments.
	paths := collectPaths(workDir, args)
	for _, p := range paths {
		for _, blocked := range s.BlockedPaths {
			if matchPathPrefix(p, blocked) {
				return &SandboxViolation{
					SandboxType: "shell",
					Rule:        "blocked_path",
					Message:     "access to blocked path: " + p,
					Severity:    "block",
				}
			}
		}
	}

	// If an allowed path list is configured, working directory must be within one.
	if len(s.AllowedPaths) > 0 && workDir != "" {
		allowed := false
		for _, a := range s.AllowedPaths {
			if matchPathPrefix(workDir, a) {
				allowed = true
				break
			}
		}
		if !allowed {
			return &SandboxViolation{
				SandboxType: "shell",
				Rule:        "allowed_path",
				Message:     "working directory not in allowed paths: " + workDir,
				Severity:    "block",
			}
		}
	}

	return nil
}

// ShouldRedactEnv returns true if the given environment variable name matches
// any of the configured redaction patterns.
func (s *ShellSandbox) ShouldRedactEnv(envName string) bool {
	upper := strings.ToUpper(envName)
	for _, pattern := range s.EnvRedactions {
		if matchGlob(upper, strings.ToUpper(pattern)) {
			return true
		}
	}
	return false
}

// baseCommand extracts the basename from a potentially absolute path.
func baseCommand(cmd string) string {
	if i := strings.LastIndex(cmd, "/"); i >= 0 {
		return cmd[i+1:]
	}
	return cmd
}

// matchGlob performs a simple glob match supporting * wildcards.
func matchGlob(value, pattern string) bool {
	matched, _ := filepath.Match(pattern, value)
	return matched
}

// matchPathPrefix checks whether path starts with prefix, handling trailing slashes.
func matchPathPrefix(path, prefix string) bool {
	cleanPath := filepath.Clean(path)
	cleanPrefix := filepath.Clean(prefix)
	if cleanPath == cleanPrefix {
		return true
	}
	return strings.HasPrefix(cleanPath, cleanPrefix+"/")
}

// collectPaths gathers filesystem paths from the working directory and arguments.
func collectPaths(workDir string, args []string) []string {
	var paths []string
	if workDir != "" {
		paths = append(paths, workDir)
	}
	for _, arg := range args {
		// Skip flags.
		if strings.HasPrefix(arg, "-") {
			continue
		}
		// Include arguments that look like paths.
		if strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") || strings.Contains(arg, "/") {
			paths = append(paths, arg)
		}
	}
	return paths
}
