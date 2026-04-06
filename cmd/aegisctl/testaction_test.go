package main

import (
	"strings"
	"testing"
)

func TestFormatTestActionOutput_Allow(t *testing.T) {
	result := &testActionResult{
		Decision:     "allow",
		EnvelopeID:   "test-uuid-1234",
		EvidenceHash: "abc123hash",
		Message:      "Action is allowed by policy",
	}
	out := formatTestActionOutput(result)
	if !strings.Contains(out, "ALLOWED") {
		t.Fatal("expected ALLOWED in output")
	}
	if !strings.Contains(out, colorGreen) {
		t.Fatal("expected green color for allow")
	}
	if !strings.Contains(out, "test-uuid-1234") {
		t.Fatal("expected envelope ID in output")
	}
	if !strings.Contains(out, "abc123hash") {
		t.Fatal("expected evidence hash in output")
	}
	if strings.Contains(out, "Approval ID") {
		t.Fatal("should not show approval ID for allow decision")
	}
}

func TestFormatTestActionOutput_Block(t *testing.T) {
	result := &testActionResult{
		Decision:     "block",
		EnvelopeID:   "test-uuid-5678",
		EvidenceHash: "def456hash",
		Message:      "Action is blocked by policy",
	}
	out := formatTestActionOutput(result)
	if !strings.Contains(out, "BLOCKED") {
		t.Fatal("expected BLOCKED in output")
	}
	if !strings.Contains(out, colorRed) {
		t.Fatal("expected red color for block")
	}
	if strings.Contains(out, "Approval ID") {
		t.Fatal("should not show approval ID for block decision")
	}
}

func TestFormatTestActionOutput_Review(t *testing.T) {
	result := &testActionResult{
		Decision:     "review",
		EnvelopeID:   "test-uuid-9012",
		EvidenceHash: "ghi789hash",
		Message:      "Action requires human review",
		ApprovalID:   "approval-xyz",
	}
	out := formatTestActionOutput(result)
	if !strings.Contains(out, "REVIEW REQUIRED") {
		t.Fatal("expected REVIEW REQUIRED in output")
	}
	if !strings.Contains(out, colorYellow) {
		t.Fatal("expected yellow color for review")
	}
	if !strings.Contains(out, "approval-xyz") {
		t.Fatal("expected approval ID in output")
	}
}

func TestFormatTestActionOutput_LocalFallback(t *testing.T) {
	result := &testActionResult{
		Decision:     "block",
		EnvelopeID:   "local-uuid",
		EvidenceHash: "localhash",
		Message:      "Action is blocked by policy",
		Local:        true,
	}
	out := formatTestActionOutput(result)
	if !strings.Contains(out, "local evaluation") {
		t.Fatal("expected local evaluation notice in output")
	}
	if !strings.Contains(out, "BLOCKED") {
		t.Fatal("expected BLOCKED in local output")
	}
}

func TestLocalTestAction_BlockDangerousShell(t *testing.T) {
	result := localTestAction("shell", "rm", "/etc", "execute", nil)
	if result.Decision != "block" {
		t.Fatalf("expected block for dangerous shell command, got %s", result.Decision)
	}
	if result.EnvelopeID == "" {
		t.Fatal("expected non-empty envelope ID")
	}
	if result.EvidenceHash == "" {
		t.Fatal("expected non-empty evidence hash")
	}
}

func TestLocalTestAction_AllowReadOp(t *testing.T) {
	result := localTestAction("git", "list_repos", "github.com/org", "read", nil)
	if result.Decision != "allow" {
		t.Fatalf("expected allow for read list operation, got %s", result.Decision)
	}
}

func TestLocalTestAction_ReviewGitPush(t *testing.T) {
	result := localTestAction("git", "push", "main", "deploy", nil)
	if result.Decision != "review" {
		t.Fatalf("expected review for git push, got %s", result.Decision)
	}
	if result.ApprovalID == "" {
		t.Fatal("expected approval ID for review decision")
	}
}
