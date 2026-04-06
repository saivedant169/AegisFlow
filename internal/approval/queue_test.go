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
