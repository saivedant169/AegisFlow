package shellgate

import "strings"

// DangerousPatterns is the built-in list of dangerous shell command patterns.
var DangerousPatterns = []dangerousPattern{
	{cmd: "rm", argsContain: "-rf /", description: "recursive force delete root"},
	{cmd: "rm", argsContain: "-rf ~", description: "recursive force delete home"},
	{cmd: "rm", argsContain: "-rf *", description: "recursive force delete wildcard"},
	{cmd: "chmod", argsContain: "777", description: "world-writable permissions"},
	{cmd: "dd", argsContain: "if=/dev", description: "raw disk read"},
	{cmd: "mkfs", argsContain: "", description: "format filesystem"},
	{cmd: "shutdown", argsContain: "", description: "system shutdown"},
	{cmd: "reboot", argsContain: "", description: "system reboot"},
	{cmd: "init", argsContain: "0", description: "system halt"},
}

// dangerousPattern describes a single dangerous command pattern.
type dangerousPattern struct {
	cmd         string
	argsContain string
	description string
}

// IsDangerous checks whether the given command and arguments match any known
// dangerous pattern. It also detects fork bombs and writes to device files.
func IsDangerous(cmd string, args []string) bool {
	fullArgs := strings.Join(args, " ")
	fullCommand := cmd + " " + fullArgs

	// Fork bomb detection
	if strings.Contains(fullCommand, ":(){ :|:&") {
		return true
	}

	// Redirect to block device
	if strings.Contains(fullCommand, "> /dev/sda") {
		return true
	}

	base := baseCommand(cmd)

	for _, p := range DangerousPatterns {
		if base != p.cmd {
			continue
		}
		// If the pattern has no args requirement, the command name alone is enough.
		if p.argsContain == "" {
			return true
		}
		if strings.Contains(fullArgs, p.argsContain) {
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
