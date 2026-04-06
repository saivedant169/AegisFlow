package sandbox

import "fmt"

// SandboxViolation describes a constraint that was violated during sandbox validation.
type SandboxViolation struct {
	SandboxType string `json:"sandbox_type"` // shell, sql, http, git
	Rule        string `json:"rule"`         // which constraint was violated
	Message     string `json:"message"`
	Severity    string `json:"severity"` // warning, block
}

// Error implements the error interface.
func (v *SandboxViolation) Error() string {
	return fmt.Sprintf("[%s:%s] %s (%s)", v.SandboxType, v.Rule, v.Message, v.Severity)
}
