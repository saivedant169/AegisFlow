package integrations

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/approval"
	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func testItem() *approval.ApprovalItem {
	env := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "agent-1", SessionID: "sess-1", TenantID: "t1"},
		"deploy service",
		envelope.ProtocolHTTP,
		"kubectl",
		"prod-cluster",
		envelope.CapDeploy,
	)
	env.EvidenceHash = "abc123hash"
	env.Parameters["pr_number"] = "42"

	return &approval.ApprovalItem{
		ID:          env.ID,
		Envelope:    env,
		Status:      approval.StatusPending,
		SubmittedAt: time.Now().UTC(),
	}
}

func TestGitHubNotifyReview(t *testing.T) {
	var receivedBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/repos/owner/repo/issues/42/comments") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or wrong auth header: %s", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	notifier := NewGitHubNotifier("test-token", srv.URL, "owner/repo")
	item := testItem()

	if err := notifier.NotifyReview(item); err != nil {
		t.Fatalf("NotifyReview: %v", err)
	}

	body := receivedBody["body"]
	if !strings.Contains(body, "kubectl") {
		t.Error("comment should contain tool name")
	}
	if !strings.Contains(body, "prod-cluster") {
		t.Error("comment should contain target")
	}
	if !strings.Contains(body, "deploy") {
		t.Error("comment should contain capability")
	}
	if !strings.Contains(body, "abc123hash") {
		t.Error("comment should contain evidence hash")
	}
	if !strings.Contains(body, "HIGH") {
		t.Error("destructive action should show HIGH risk")
	}
}

func TestGitHubNotifyApproved(t *testing.T) {
	var receivedBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	notifier := NewGitHubNotifier("test-token", srv.URL, "owner/repo")
	item := testItem()
	item.Status = approval.StatusApproved
	item.Reviewer = "alice"
	item.ReviewComment = "looks safe"

	if err := notifier.NotifyApproved(item); err != nil {
		t.Fatalf("NotifyApproved: %v", err)
	}

	body := receivedBody["body"]
	if !strings.Contains(body, "Approved") {
		t.Error("comment should contain 'Approved'")
	}
	if !strings.Contains(body, "alice") {
		t.Error("comment should contain reviewer name")
	}
	if !strings.Contains(body, "looks safe") {
		t.Error("comment should contain review comment")
	}
}

func TestGitHubNotifyDenied(t *testing.T) {
	var receivedBody map[string]string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	notifier := NewGitHubNotifier("test-token", srv.URL, "owner/repo")
	item := testItem()
	item.Status = approval.StatusDenied
	item.Reviewer = "bob"
	item.ReviewComment = "too risky"

	if err := notifier.NotifyDenied(item); err != nil {
		t.Fatalf("NotifyDenied: %v", err)
	}

	body := receivedBody["body"]
	if !strings.Contains(body, "Denied") {
		t.Error("comment should contain 'Denied'")
	}
	if !strings.Contains(body, "bob") {
		t.Error("comment should contain reviewer name")
	}
	if !strings.Contains(body, "too risky") {
		t.Error("comment should contain review comment")
	}
}

func TestGitHubAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	notifier := NewGitHubNotifier("bad-token", srv.URL, "owner/repo")
	item := testItem()

	err := notifier.NotifyReview(item)
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention status code, got: %v", err)
	}
}
