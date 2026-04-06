package behavioral

import (
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func makeEnv(id, tool, target string, cap envelope.Capability, proto envelope.Protocol, ts time.Time) *envelope.ActionEnvelope {
	return &envelope.ActionEnvelope{
		ID:                  id,
		Timestamp:           ts,
		Actor:               envelope.ActorInfo{Type: "agent", ID: "a1", SessionID: "sess-1"},
		Task:                "test-task",
		Protocol:            proto,
		Tool:                tool,
		Target:              target,
		Parameters:          map[string]any{},
		RequestedCapability: cap,
		PolicyDecision:      envelope.DecisionAllow,
	}
}

func TestExfiltrationDetection(t *testing.T) {
	sa := NewSessionAnalyzer("sess-1", DefaultRules(), 0, 0)
	now := time.Now().UTC()

	// Read .env file
	sa.RecordAction(makeEnv("e1", "file.read", ".env", envelope.CapRead, envelope.ProtocolShell, now))
	// curl to external
	sa.RecordAction(makeEnv("e2", "curl", "https://evil.com", envelope.CapWrite, envelope.ProtocolHTTP, now.Add(time.Second)))

	alerts := sa.Analyze()
	found := false
	for _, a := range alerts {
		if a.Rule == "exfiltration_pattern" {
			found = true
			if a.Severity != "critical" {
				t.Errorf("expected severity critical, got %s", a.Severity)
			}
		}
	}
	if !found {
		t.Fatal("expected exfiltration_pattern alert")
	}
}

func TestPrivilegeEscalation(t *testing.T) {
	sa := NewSessionAnalyzer("sess-2", DefaultRules(), 0, 0)
	now := time.Now().UTC()

	// Edit workflow file
	sa.RecordAction(makeEnv("e1", "file.write", ".github/workflows/ci.yaml", envelope.CapWrite, envelope.ProtocolGit, now))
	// Push
	sa.RecordAction(makeEnv("e2", "git.push", "origin/main", envelope.CapDeploy, envelope.ProtocolGit, now.Add(time.Second)))

	alerts := sa.Analyze()
	found := false
	for _, a := range alerts {
		if a.Rule == "privilege_escalation" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected privilege_escalation alert")
	}
}

func TestDestructiveSequence(t *testing.T) {
	sa := NewSessionAnalyzer("sess-3", DefaultRules(), 0, 0)
	now := time.Now().UTC()

	sa.RecordAction(makeEnv("e1", "file.delete", "/tmp/a", envelope.CapDelete, envelope.ProtocolShell, now))
	sa.RecordAction(makeEnv("e2", "file.delete", "/tmp/b", envelope.CapDelete, envelope.ProtocolShell, now.Add(time.Second)))
	sa.RecordAction(makeEnv("e3", "file.delete", "/tmp/c", envelope.CapDelete, envelope.ProtocolShell, now.Add(2*time.Second)))

	alerts := sa.Analyze()
	found := false
	for _, a := range alerts {
		if a.Rule == "destructive_sequence" {
			found = true
			if len(a.Actions) < 3 {
				t.Errorf("expected at least 3 actions, got %d", len(a.Actions))
			}
		}
	}
	if !found {
		t.Fatal("expected destructive_sequence alert")
	}
}

func TestSuspiciousFanOut(t *testing.T) {
	sa := NewSessionAnalyzer("sess-4", DefaultRules(), 0, 0)
	now := time.Now().UTC()

	// 10 different targets in under 1 minute
	for i := 0; i < 10; i++ {
		target := "host-" + string(rune('a'+i)) + ".example.com"
		sa.RecordAction(makeEnv("e"+string(rune('0'+i)), "http.get", target, envelope.CapRead, envelope.ProtocolHTTP, now.Add(time.Duration(i)*time.Second)))
	}

	alerts := sa.Analyze()
	found := false
	for _, a := range alerts {
		if a.Rule == "suspicious_fan_out" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected suspicious_fan_out alert")
	}
}

func TestNoAlertForNormalBehavior(t *testing.T) {
	sa := NewSessionAnalyzer("sess-5", DefaultRules(), 0, 0)
	now := time.Now().UTC()

	// Normal read operations on non-sensitive targets
	sa.RecordAction(makeEnv("e1", "file.read", "/app/data.txt", envelope.CapRead, envelope.ProtocolShell, now))
	sa.RecordAction(makeEnv("e2", "file.read", "/app/config.json", envelope.CapRead, envelope.ProtocolShell, now.Add(time.Second)))
	sa.RecordAction(makeEnv("e3", "file.write", "/app/output.txt", envelope.CapWrite, envelope.ProtocolShell, now.Add(2*time.Second)))

	alerts := sa.Analyze()
	if len(alerts) != 0 {
		t.Fatalf("expected no alerts for normal behavior, got %d: %+v", len(alerts), alerts)
	}
}

func TestSessionRiskScore(t *testing.T) {
	sa := NewSessionAnalyzer("sess-6", DefaultRules(), 0, 0)
	now := time.Now().UTC()

	// Trigger exfiltration (40 points)
	sa.RecordAction(makeEnv("e1", "file.read", ".env", envelope.CapRead, envelope.ProtocolShell, now))
	sa.RecordAction(makeEnv("e2", "curl", "https://evil.com", envelope.CapWrite, envelope.ProtocolHTTP, now.Add(time.Second)))

	sa.Analyze()

	score := sa.SessionRiskScore()
	if score == 0 {
		t.Fatal("expected non-zero risk score")
	}
	if score > 100 {
		t.Fatalf("expected risk score <= 100, got %d", score)
	}
}

func TestKillSwitchThreshold(t *testing.T) {
	sa := NewSessionAnalyzer("sess-7", DefaultRules(), 80, 0)
	now := time.Now().UTC()

	// Trigger exfiltration (40 points)
	sa.RecordAction(makeEnv("e1", "file.read", ".env", envelope.CapRead, envelope.ProtocolShell, now))
	sa.RecordAction(makeEnv("e2", "curl", "https://evil.com", envelope.CapWrite, envelope.ProtocolHTTP, now.Add(time.Second)))
	// Trigger destructive sequence (25 points)
	sa.RecordAction(makeEnv("e3", "file.delete", "/a", envelope.CapDelete, envelope.ProtocolShell, now.Add(2*time.Second)))
	sa.RecordAction(makeEnv("e4", "file.delete", "/b", envelope.CapDelete, envelope.ProtocolShell, now.Add(3*time.Second)))
	sa.RecordAction(makeEnv("e5", "file.delete", "/c", envelope.CapDelete, envelope.ProtocolShell, now.Add(4*time.Second)))
	// Trigger credential abuse (35 points) -- read secret + 2 external calls
	sa.RecordAction(makeEnv("e6", "vault.read", "secret/db", envelope.CapRead, envelope.ProtocolHTTP, now.Add(5*time.Second)))
	sa.RecordAction(makeEnv("e7", "http.post", "https://a.com", envelope.CapWrite, envelope.ProtocolHTTP, now.Add(6*time.Second)))
	sa.RecordAction(makeEnv("e8", "http.post", "https://b.com", envelope.CapWrite, envelope.ProtocolHTTP, now.Add(7*time.Second)))

	sa.Analyze()

	if !sa.Blocked() {
		t.Fatalf("expected session to be blocked (kill switch), score=%d", sa.SessionRiskScore())
	}
}

func TestRegistryGetOrCreate(t *testing.T) {
	reg := NewRegistry(DefaultRules(), 80, 60)
	sa1 := reg.GetOrCreate("sess-a")
	sa2 := reg.GetOrCreate("sess-a")
	if sa1 != sa2 {
		t.Fatal("expected same analyzer for same session ID")
	}

	sa3 := reg.GetOrCreate("sess-b")
	if sa1 == sa3 {
		t.Fatal("expected different analyzer for different session ID")
	}

	ids := reg.ListSessions()
	if len(ids) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(ids))
	}
}
