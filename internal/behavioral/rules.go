package behavioral

import (
	"strings"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// Rule is a behavioral detection rule that inspects action history for risky patterns.
type Rule interface {
	Name() string
	Description() string
	Detect(history []envelope.ActionEnvelope) *BehaviorAlert
}

// ExfiltrationPattern detects when a session reads a sensitive resource then
// POSTs data to an external host.
type ExfiltrationPattern struct{}

func (r ExfiltrationPattern) Name() string { return "exfiltration_pattern" }
func (r ExfiltrationPattern) Description() string {
	return "Read sensitive resource then POST to external host"
}

func (r ExfiltrationPattern) Detect(history []envelope.ActionEnvelope) *BehaviorAlert {
	sensitiveTargets := []string{".env", "secret", "credential", "password", "token", "key", "private"}
	var readSensitiveIDs []string
	sawSensitiveRead := false

	for i, env := range history {
		if env.RequestedCapability == envelope.CapRead {
			for _, s := range sensitiveTargets {
				if strings.Contains(strings.ToLower(env.Target), s) ||
					strings.Contains(strings.ToLower(env.Tool), s) {
					sawSensitiveRead = true
					readSensitiveIDs = append(readSensitiveIDs, env.ID)
					break
				}
			}
		}

		if sawSensitiveRead && i > 0 {
			if isExternalWrite(env) {
				ids := append(readSensitiveIDs, env.ID)
				return &BehaviorAlert{
					Rule:      r.Name(),
					Severity:  "critical",
					Message:   "Session read sensitive resource then sent data to external host",
					Actions:   ids,
					RiskScore: 40,
					Timestamp: time.Now().UTC(),
				}
			}
		}
	}
	return nil
}

// PrivilegeEscalation detects when a session edits workflow/CI files then pushes or deploys.
type PrivilegeEscalation struct{}

func (r PrivilegeEscalation) Name() string { return "privilege_escalation" }
func (r PrivilegeEscalation) Description() string {
	return "Edit workflow or CI configuration then push or deploy"
}

func (r PrivilegeEscalation) Detect(history []envelope.ActionEnvelope) *BehaviorAlert {
	ciTargets := []string{"workflow", ".github", "ci", "pipeline", "jenkinsfile", "gitlab-ci", "circleci"}
	var editIDs []string
	sawCIEdit := false

	for _, env := range history {
		if env.RequestedCapability == envelope.CapWrite {
			for _, t := range ciTargets {
				if strings.Contains(strings.ToLower(env.Target), t) ||
					strings.Contains(strings.ToLower(env.Tool), t) {
					sawCIEdit = true
					editIDs = append(editIDs, env.ID)
					break
				}
			}
		}

		if sawCIEdit && (env.RequestedCapability == envelope.CapDeploy ||
			strings.Contains(strings.ToLower(env.Tool), "push") ||
			strings.Contains(strings.ToLower(env.Tool), "deploy")) {
			ids := append(editIDs, env.ID)
			return &BehaviorAlert{
				Rule:      r.Name(),
				Severity:  "critical",
				Message:   "Session edited CI/workflow config then pushed or deployed",
				Actions:   ids,
				RiskScore: 35,
				Timestamp: time.Now().UTC(),
			}
		}
	}
	return nil
}

// CredentialAbuse detects when a session reads secrets then makes multiple external calls.
type CredentialAbuse struct{}

func (r CredentialAbuse) Name() string { return "credential_abuse" }
func (r CredentialAbuse) Description() string {
	return "Read secrets then make multiple external calls"
}

func (r CredentialAbuse) Detect(history []envelope.ActionEnvelope) *BehaviorAlert {
	secretTargets := []string{"secret", "credential", "password", "token", "api_key", ".env"}
	var readIDs []string
	sawSecretRead := false
	externalCallCount := 0
	var externalIDs []string

	for _, env := range history {
		if env.RequestedCapability == envelope.CapRead {
			for _, s := range secretTargets {
				if strings.Contains(strings.ToLower(env.Target), s) ||
					strings.Contains(strings.ToLower(env.Tool), s) {
					sawSecretRead = true
					readIDs = append(readIDs, env.ID)
					break
				}
			}
		}

		if sawSecretRead && isExternalCall(env) {
			externalCallCount++
			externalIDs = append(externalIDs, env.ID)
		}
	}

	if sawSecretRead && externalCallCount >= 2 {
		ids := append(readIDs, externalIDs...)
		return &BehaviorAlert{
			Rule:      r.Name(),
			Severity:  "critical",
			Message:   "Session read secrets then made multiple external calls",
			Actions:   ids,
			RiskScore: 35,
			Timestamp: time.Now().UTC(),
		}
	}
	return nil
}

// DestructiveSequence detects multiple delete operations in sequence.
type DestructiveSequence struct {
	Threshold int // number of consecutive deletes; defaults to 3
}

func (r DestructiveSequence) Name() string { return "destructive_sequence" }
func (r DestructiveSequence) Description() string {
	return "Multiple delete operations in sequence"
}

func (r DestructiveSequence) Detect(history []envelope.ActionEnvelope) *BehaviorAlert {
	threshold := r.Threshold
	if threshold == 0 {
		threshold = 3
	}

	var consecutive []string
	for _, env := range history {
		if env.RequestedCapability == envelope.CapDelete {
			consecutive = append(consecutive, env.ID)
		} else {
			if len(consecutive) >= threshold {
				break
			}
			consecutive = nil
		}
	}

	if len(consecutive) >= threshold {
		return &BehaviorAlert{
			Rule:      r.Name(),
			Severity:  "warning",
			Message:   "Multiple consecutive delete operations detected",
			Actions:   consecutive,
			RiskScore: 25,
			Timestamp: time.Now().UTC(),
		}
	}
	return nil
}

// SuspiciousFanOut detects a single session hitting many different targets rapidly.
type SuspiciousFanOut struct {
	MaxTargets    int // threshold; defaults to 10
	WindowSeconds int // time window in seconds; defaults to 60
}

func (r SuspiciousFanOut) Name() string { return "suspicious_fan_out" }
func (r SuspiciousFanOut) Description() string {
	return "Single session hitting many different targets rapidly"
}

func (r SuspiciousFanOut) Detect(history []envelope.ActionEnvelope) *BehaviorAlert {
	maxTargets := r.MaxTargets
	if maxTargets == 0 {
		maxTargets = 10
	}
	windowSec := r.WindowSeconds
	if windowSec == 0 {
		windowSec = 60
	}
	window := time.Duration(windowSec) * time.Second

	if len(history) == 0 {
		return nil
	}

	// Two-pointer sliding window over the time-ordered history: advance right,
	// shrink left while the span exceeds the window, and track the count of
	// distinct targets currently inside [left, right]. Each action enters and
	// leaves the window once, so this is O(len(history)) instead of the
	// O(len(history)^2) restart-from-every-start scan.
	counts := make(map[string]int)
	left := 0
	for right := 0; right < len(history); right++ {
		counts[history[right].Target]++
		for history[right].Timestamp.Sub(history[left].Timestamp) > window {
			t := history[left].Target
			if counts[t]--; counts[t] == 0 {
				delete(counts, t)
			}
			left++
		}
		if len(counts) >= maxTargets {
			ids := make([]string, 0, right-left+1)
			for k := left; k <= right; k++ {
				ids = append(ids, history[k].ID)
			}
			return &BehaviorAlert{
				Rule:      r.Name(),
				Severity:  "warning",
				Message:   "Session contacted many distinct targets in a short window",
				Actions:   ids,
				RiskScore: 20,
				Timestamp: time.Now().UTC(),
			}
		}
	}
	return nil
}

// RepeatedEscalation detects multiple review/approval requests in a short time.
type RepeatedEscalation struct {
	MaxRequests   int // threshold; defaults to 3
	WindowSeconds int // time window in seconds; defaults to 300 (5 min)
}

func (r RepeatedEscalation) Name() string { return "repeated_escalation" }
func (r RepeatedEscalation) Description() string {
	return "Multiple review or approval requests in short time"
}

func (r RepeatedEscalation) Detect(history []envelope.ActionEnvelope) *BehaviorAlert {
	maxReqs := r.MaxRequests
	if maxReqs == 0 {
		maxReqs = 3
	}
	windowSec := r.WindowSeconds
	if windowSec == 0 {
		windowSec = 300
	}
	window := time.Duration(windowSec) * time.Second

	// Collect escalation actions (approve capability or review decisions).
	var escalations []envelope.ActionEnvelope
	for _, env := range history {
		if env.RequestedCapability == envelope.CapApprove ||
			env.PolicyDecision == envelope.DecisionReview {
			escalations = append(escalations, env)
		}
	}

	if len(escalations) < maxReqs {
		return nil
	}

	// Two-pointer sliding window over escalation events: keep [left, right]
	// within the time window and alert once it holds maxReqs events. O(k)
	// instead of restarting the inner count from every start.
	left := 0
	for right := 0; right < len(escalations); right++ {
		for escalations[right].Timestamp.Sub(escalations[left].Timestamp) > window {
			left++
		}
		if right-left+1 >= maxReqs {
			ids := make([]string, 0, right-left+1)
			for k := left; k <= right; k++ {
				ids = append(ids, escalations[k].ID)
			}
			return &BehaviorAlert{
				Rule:      r.Name(),
				Severity:  "warning",
				Message:   "Session made multiple review/approval requests in a short window",
				Actions:   ids,
				RiskScore: 15,
				Timestamp: time.Now().UTC(),
			}
		}
	}
	return nil
}

// DefaultRules returns the built-in set of behavioral detection rules.
func DefaultRules() []Rule {
	return []Rule{
		ExfiltrationPattern{},
		PrivilegeEscalation{},
		CredentialAbuse{},
		DestructiveSequence{},
		SuspiciousFanOut{},
		RepeatedEscalation{},
	}
}

// isExternalWrite checks if an action looks like sending data to an external host.
func isExternalWrite(env envelope.ActionEnvelope) bool {
	if env.Protocol == envelope.ProtocolHTTP &&
		(env.RequestedCapability == envelope.CapWrite || env.RequestedCapability == envelope.CapExecute) {
		return true
	}
	tool := strings.ToLower(env.Tool)
	if strings.Contains(tool, "curl") || strings.Contains(tool, "http") ||
		strings.Contains(tool, "post") || strings.Contains(tool, "fetch") ||
		strings.Contains(tool, "request") {
		if env.RequestedCapability == envelope.CapWrite || env.RequestedCapability == envelope.CapExecute {
			return true
		}
	}
	return false
}

// isExternalCall checks if an action targets an external service.
func isExternalCall(env envelope.ActionEnvelope) bool {
	if env.Protocol == envelope.ProtocolHTTP {
		return true
	}
	tool := strings.ToLower(env.Tool)
	return strings.Contains(tool, "curl") || strings.Contains(tool, "http") ||
		strings.Contains(tool, "fetch") || strings.Contains(tool, "request") ||
		strings.Contains(tool, "webhook")
}
