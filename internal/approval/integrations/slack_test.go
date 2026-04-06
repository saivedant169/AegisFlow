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

func slackTestItem() *approval.ApprovalItem {
	env := envelope.NewEnvelope(
		envelope.ActorInfo{Type: "agent", ID: "agent-2", SessionID: "sess-2", TenantID: "t2"},
		"delete table",
		envelope.ProtocolSQL,
		"psql",
		"users_table",
		envelope.CapDelete,
	)
	env.EvidenceHash = "deadbeef"

	return &approval.ApprovalItem{
		ID:          env.ID,
		Envelope:    env,
		Status:      approval.StatusPending,
		SubmittedAt: time.Now().UTC(),
	}
}

func TestSlackNotifyReview(t *testing.T) {
	var receivedPayload slackPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := NewSlackNotifier(srv.URL, "https://admin.example.com")
	item := slackTestItem()

	if err := notifier.NotifyReview(item); err != nil {
		t.Fatalf("NotifyReview: %v", err)
	}

	if len(receivedPayload.Blocks) < 4 {
		t.Fatalf("expected at least 4 blocks, got %d", len(receivedPayload.Blocks))
	}

	// Header block should indicate urgency (destructive = URGENT)
	header := receivedPayload.Blocks[0]
	if header.Type != "header" {
		t.Errorf("first block should be header, got %s", header.Type)
	}
	if header.Text == nil || !strings.Contains(header.Text.Text, "URGENT") {
		t.Error("destructive action should show URGENT in header")
	}

	// Section with fields
	fields := receivedPayload.Blocks[1]
	if len(fields.Fields) < 4 {
		t.Fatalf("expected at least 4 fields, got %d", len(fields.Fields))
	}

	fieldTexts := ""
	for _, f := range fields.Fields {
		fieldTexts += f.Text + " "
	}
	if !strings.Contains(fieldTexts, "psql") {
		t.Error("fields should contain tool name")
	}
	if !strings.Contains(fieldTexts, "users_table") {
		t.Error("fields should contain target")
	}
	if !strings.Contains(fieldTexts, "deadbeef") {
		t.Error("fields should contain evidence hash")
	}

	// Actions block with buttons
	actions := receivedPayload.Blocks[3]
	if actions.Type != "actions" {
		t.Errorf("fourth block should be actions, got %s", actions.Type)
	}
	if len(actions.Elements) != 2 {
		t.Fatalf("expected 2 buttons, got %d", len(actions.Elements))
	}
	if !strings.Contains(actions.Elements[0].URL, "/approve") {
		t.Error("first button should link to approve endpoint")
	}
	if !strings.Contains(actions.Elements[1].URL, "/deny") {
		t.Error("second button should link to deny endpoint")
	}
}

func TestSlackNotifyApproved(t *testing.T) {
	var receivedPayload slackPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	notifier := NewSlackNotifier(srv.URL, "https://admin.example.com")
	item := slackTestItem()
	item.Status = approval.StatusApproved
	item.Reviewer = "carol"
	item.ReviewComment = "approved after review"

	if err := notifier.NotifyApproved(item); err != nil {
		t.Fatalf("NotifyApproved: %v", err)
	}

	if len(receivedPayload.Blocks) < 1 {
		t.Fatal("expected at least 1 block")
	}
	text := receivedPayload.Blocks[0].Text.Text
	if !strings.Contains(text, "Approved") {
		t.Error("message should contain 'Approved'")
	}
	if !strings.Contains(text, "carol") {
		t.Error("message should contain reviewer name")
	}
}

func TestSlackAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	notifier := NewSlackNotifier(srv.URL, "https://admin.example.com")
	item := slackTestItem()

	err := notifier.NotifyReview(item)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status code, got: %v", err)
	}
}
