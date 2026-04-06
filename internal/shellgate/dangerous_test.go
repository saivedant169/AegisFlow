package shellgate

import "testing"

func TestDangerousPatterns(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		args []string
	}{
		{"rm -rf /", "rm", []string{"-rf", "/"}},
		{"rm -rf ~", "rm", []string{"-rf", "~"}},
		{"rm -rf *", "rm", []string{"-rf", "*"}},
		{"chmod 777", "chmod", []string{"777", "/some/path"}},
		{"dd if=/dev", "dd", []string{"if=/dev/zero", "of=/tmp/x"}},
		{"mkfs", "mkfs", []string{"-t", "ext4", "/dev/sdb1"}},
		{"fork bomb", "bash", []string{"-c", ":(){ :|:& };:"}},
		{"> /dev/sda", "bash", []string{"-c", "echo foo > /dev/sda"}},
		{"shutdown", "shutdown", []string{"-h", "now"}},
		{"reboot", "reboot", nil},
		{"init 0", "init", []string{"0"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !IsDangerous(tt.cmd, tt.args) {
				t.Errorf("expected IsDangerous(%q, %v) = true", tt.cmd, tt.args)
			}
		})
	}
}

func TestSafeCommandsNotFlagged(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		args []string
	}{
		{"ls", "ls", []string{"-la"}},
		{"cat", "cat", []string{"file.txt"}},
		{"pytest", "pytest", []string{"tests/"}},
		{"go test", "go", []string{"test", "./..."}},
		{"git status", "git", []string{"status"}},
		{"echo hello", "echo", []string{"hello"}},
		{"mkdir", "mkdir", []string{"-p", "/tmp/test"}},
		{"rm single file", "rm", []string{"file.txt"}},
		{"chmod 644", "chmod", []string{"644", "file.txt"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsDangerous(tt.cmd, tt.args) {
				t.Errorf("expected IsDangerous(%q, %v) = false", tt.cmd, tt.args)
			}
		})
	}
}
