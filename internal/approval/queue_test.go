package approval

import (
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func testEnv(tool string) *envelope.ActionEnvelope {
	return &envelope.ActionEnvelope{
		ID:                  "env-" + tool,
		Timestamp:           time.Now().UTC(),
		Actor:               envelope.ActorInfo{Type: "agent", ID: "agent-1", TenantID: "t1", SessionID: "s1"},
		Task:                "test-task",
		Protocol:            envelope.ProtocolGit,
		Tool:                tool,
		Target:              "repo/main",
		RequestedCapability: envelope.CapWrite,
		PolicyDecision:      envelope.DecisionReview,
	}
}

func TestSubmitAndList(t *testing.T) {
	q := NewQueue(100)
	env := testEnv("github.create_pr")

	id, err := q.Submit(env)
	if err != nil {
		t.Fatalf("submit failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	pending := q.Pending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].Envelope.Tool != "github.create_pr" {
		t.Fatalf("wrong tool: %s", pending[0].Envelope.Tool)
	}
}

func TestApprove(t *testing.T) {
	q := NewQueue(100)
	env := testEnv("github.create_pr")
	id, _ := q.Submit(env)

	item, err := q.Approve(id, "admin-user", "looks good")
	if err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if item.Status != StatusApproved {
		t.Fatalf("expected approved, got %s", item.Status)
	}
	if item.Reviewer != "admin-user" {
		t.Fatalf("wrong reviewer: %s", item.Reviewer)
	}

	// Should no longer be in pending
	if len(q.Pending()) != 0 {
		t.Fatal("expected 0 pending after approval")
	}
}

func TestDeny(t *testing.T) {
	q := NewQueue(100)
	env := testEnv("github.merge_pr")
	id, _ := q.Submit(env)

	item, err := q.Deny(id, "admin-user", "too risky")
	if err != nil {
		t.Fatalf("deny failed: %v", err)
	}
	if item.Status != StatusDenied {
		t.Fatalf("expected denied, got %s", item.Status)
	}
	if item.ReviewComment != "too risky" {
		t.Fatalf("wrong comment: %s", item.ReviewComment)
	}
}

func TestApproveNotFound(t *testing.T) {
	q := NewQueue(100)
	_, err := q.Approve("nonexistent", "admin", "")
	if err == nil {
		t.Fatal("expected error for nonexistent ID")
	}
}

func TestDoubleApprove(t *testing.T) {
	q := NewQueue(100)
	env := testEnv("tool")
	id, _ := q.Submit(env)
	q.Approve(id, "admin", "ok")

	_, err := q.Approve(id, "admin", "again")
	if err == nil {
		t.Fatal("expected error for already-reviewed item")
	}
}

func TestQueueFull(t *testing.T) {
	q := NewQueue(2)
	q.Submit(testEnv("t1"))
	q.Submit(testEnv("t2"))
	_, err := q.Submit(testEnv("t3"))
	if err == nil {
		t.Fatal("expected error when queue is full")
	}
}

func TestGetByID(t *testing.T) {
	q := NewQueue(100)
	env := testEnv("tool")
	id, _ := q.Submit(env)

	item, err := q.Get(id)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if item.ID != id {
		t.Fatalf("wrong ID: %s", item.ID)
	}
}

func TestIsApprovedForTool_Approved(t *testing.T) {
	q := NewQueue(100)
	env := testEnv("github.create_pr")
	id, _ := q.Submit(env)
	q.Approve(id, "admin", "ok")

	if !q.IsApprovedForTool("github.create_pr") {
		t.Fatal("expected IsApprovedForTool to return true for approved tool")
	}
}

func TestIsApprovedForTool_NotApproved(t *testing.T) {
	q := NewQueue(100)

	// Empty history
	if q.IsApprovedForTool("github.create_pr") {
		t.Fatal("expected false for empty history")
	}

	// Pending but not yet approved
	q.Submit(testEnv("github.create_pr"))
	if q.IsApprovedForTool("github.create_pr") {
		t.Fatal("expected false for pending (not approved) tool")
	}
}

func TestIsApprovedForTool_DeniedNotApproved(t *testing.T) {
	q := NewQueue(100)
	env := testEnv("github.delete_repo")
	id, _ := q.Submit(env)
	q.Deny(id, "admin", "too risky")

	if q.IsApprovedForTool("github.delete_repo") {
		t.Fatal("expected false for denied tool")
	}
}

func TestIsApprovedForTool_DifferentTool(t *testing.T) {
	q := NewQueue(100)
	env := testEnv("github.create_pr")
	id, _ := q.Submit(env)
	q.Approve(id, "admin", "ok")

	if q.IsApprovedForTool("github.delete_repo") {
		t.Fatal("expected false for different tool name")
	}
}

func TestCleanupExpired(t *testing.T) {
	q := NewQueue(100)
	q.Timeout = 1 * time.Millisecond // expire immediately

	q.Submit(testEnv("tool-1"))
	q.Submit(testEnv("tool-2"))

	// Let them expire
	time.Sleep(5 * time.Millisecond)

	expired := q.CleanupExpired()
	if expired != 2 {
		t.Fatalf("expected 2 expired, got %d", expired)
	}
	if len(q.Pending()) != 0 {
		t.Fatalf("expected 0 pending after cleanup, got %d", len(q.Pending()))
	}

	// Verify they're in history as expired
	hist := q.History(10)
	for _, item := range hist {
		if item.Status != StatusExpired {
			t.Fatalf("expected expired status, got %s", item.Status)
		}
	}
}

func TestCleanupExpiredSkipsActive(t *testing.T) {
	q := NewQueue(100)
	q.Timeout = 1 * time.Hour // won't expire

	q.Submit(testEnv("tool-1"))

	expired := q.CleanupExpired()
	if expired != 0 {
		t.Fatalf("expected 0 expired, got %d", expired)
	}
	if len(q.Pending()) != 1 {
		t.Fatalf("expected 1 still pending, got %d", len(q.Pending()))
	}
}

func TestHistory(t *testing.T) {
	q := NewQueue(100)
	id1, _ := q.Submit(testEnv("t1"))
	id2, _ := q.Submit(testEnv("t2"))
	q.Approve(id1, "admin", "ok")
	q.Deny(id2, "admin", "no")

	hist := q.History(10)
	if len(hist) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(hist))
	}
}
