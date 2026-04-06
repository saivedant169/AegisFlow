package resilience

import "testing"

func TestDegradationManager_DefaultNormal(t *testing.T) {
	dm := NewDegradationManager()
	modes := dm.All()
	for _, m := range modes {
		if m.Mode != "normal" {
			t.Fatalf("expected normal mode for %s, got %s", m.Component, m.Mode)
		}
	}
}

func TestDegradationManager_PolicyEngine_FailClosed(t *testing.T) {
	dm := NewDegradationManager()
	dm.SetDegraded("PolicyEngine")
	m := dm.Get("PolicyEngine")
	if m.Mode != "degraded" {
		t.Fatalf("expected degraded, got %s", m.Mode)
	}
	if m.Behavior != "fail-closed: all requests blocked" {
		t.Fatalf("unexpected behavior: %s", m.Behavior)
	}
}

func TestDegradationManager_EvidenceChain_BufferInMemory(t *testing.T) {
	dm := NewDegradationManager()
	dm.SetDegraded("EvidenceChain")
	m := dm.Get("EvidenceChain")
	if m.Mode != "degraded" {
		t.Fatalf("expected degraded, got %s", m.Mode)
	}
	if m.Behavior != "buffering in memory, reduced proof guarantees" {
		t.Fatalf("unexpected behavior: %s", m.Behavior)
	}
}

func TestDegradationManager_ApprovalQueue_AutoDeny(t *testing.T) {
	dm := NewDegradationManager()
	dm.SetDegraded("ApprovalQueue")
	m := dm.Get("ApprovalQueue")
	if m.Behavior != "auto-deny all review actions" {
		t.Fatalf("unexpected behavior: %s", m.Behavior)
	}
}

func TestDegradationManager_CredentialBroker_DenyIssuance(t *testing.T) {
	dm := NewDegradationManager()
	dm.SetDegraded("CredentialBroker")
	m := dm.Get("CredentialBroker")
	if m.Behavior != "deny all credential issuance" {
		t.Fatalf("unexpected behavior: %s", m.Behavior)
	}
}

func TestDegradationManager_AuditLog_BufferFlush(t *testing.T) {
	dm := NewDegradationManager()
	dm.SetDegraded("AuditLog")
	m := dm.Get("AuditLog")
	if m.Behavior != "buffering in memory, flush on recovery" {
		t.Fatalf("unexpected behavior: %s", m.Behavior)
	}
}

func TestDegradationManager_Emergency(t *testing.T) {
	dm := NewDegradationManager()
	dm.SetEmergency("PolicyEngine")
	m := dm.Get("PolicyEngine")
	if m.Mode != "emergency" {
		t.Fatalf("expected emergency, got %s", m.Mode)
	}
}

func TestDegradationManager_Recovery(t *testing.T) {
	dm := NewDegradationManager()
	dm.SetDegraded("AuditLog")
	dm.SetNormal("AuditLog")
	m := dm.Get("AuditLog")
	if m.Mode != "normal" {
		t.Fatalf("expected normal after recovery, got %s", m.Mode)
	}
	if m.SafetyImpact != "none" {
		t.Fatalf("expected no safety impact after recovery, got %s", m.SafetyImpact)
	}
}

func TestDegradationManager_UnknownComponent(t *testing.T) {
	dm := NewDegradationManager()
	dm.SetDegraded("Custom")
	m := dm.Get("Custom")
	if m == nil {
		t.Fatal("expected non-nil mode for unknown component")
	}
	if m.Mode != "degraded" {
		t.Fatalf("expected degraded, got %s", m.Mode)
	}
}
