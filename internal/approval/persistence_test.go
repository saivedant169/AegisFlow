package approval

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
	"github.com/saivedant169/AegisFlow/internal/state"
)

func TestPersistentQueueRestoresPendingAndHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	firstState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewPersistentQueue(100, firstState.DB(), []byte("persistence-key"))
	if err != nil {
		t.Fatal(err)
	}
	pendingID, err := first.Submit(testEnv("repo.pending"))
	if err != nil {
		t.Fatal(err)
	}
	approvedID, err := first.Submit(testEnv("repo.approved"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Approve(approvedID, "reviewer", "scope checked"); err != nil {
		t.Fatal(err)
	}
	if err := firstState.Close(); err != nil {
		t.Fatal(err)
	}

	secondState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondState.Close()
	second, err := NewPersistentQueue(100, secondState.DB(), []byte("persistence-key"))
	if err != nil {
		t.Fatalf("restore queue: %v", err)
	}
	if _, err := second.Get(pendingID); err != nil {
		t.Fatalf("pending item missing after restart: %v", err)
	}
	approved, err := second.Get(approvedID)
	if err != nil {
		t.Fatalf("approved item missing after restart: %v", err)
	}
	if approved.Status != StatusApproved || approved.Reviewer != "reviewer" {
		t.Fatalf("unexpected restored approval: %+v", approved)
	}
}

func TestPersistentQueueApprovalStaysSingleUseAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	firstState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewPersistentQueue(100, firstState.DB(), []byte("single-use-key"))
	if err != nil {
		t.Fatal(err)
	}
	original := envWithTarget("repo.open_change", "acme/widgets")
	original.Parameters = map[string]any{"head": "fix/persisted"}
	id, err := first.Submit(original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Approve(id, "reviewer", "scope checked"); err != nil {
		t.Fatal(err)
	}
	if err := firstState.Close(); err != nil {
		t.Fatal(err)
	}

	secondState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewPersistentQueue(100, secondState.DB(), []byte("single-use-key"))
	if err != nil {
		t.Fatal(err)
	}
	retry := envWithTarget("repo.open_change", "acme/widgets")
	retry.ID = "retry-after-restart"
	retry.Parameters = map[string]any{"head": "fix/persisted"}
	if !second.ConsumeApprovalForEnvelope(retry) {
		t.Fatal("restored approval did not cover exact retry")
	}
	changed := envWithTarget("repo.open_change", "acme/widgets")
	changed.Parameters = map[string]any{"head": "fix/changed"}
	if second.ConsumeApprovalForEnvelope(changed) {
		t.Fatal("restored approval covered changed arguments")
	}
	if err := secondState.Close(); err != nil {
		t.Fatal(err)
	}

	thirdState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer thirdState.Close()
	third, err := NewPersistentQueue(100, thirdState.DB(), []byte("single-use-key"))
	if err != nil {
		t.Fatal(err)
	}
	if third.ConsumeApprovalForEnvelope(retry) {
		t.Fatal("consumed approval became reusable after restart")
	}
}

func TestPersistentQueueRestoresExpiredStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	firstState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := NewPersistentQueue(100, firstState.DB(), []byte("expiry-key"))
	if err != nil {
		t.Fatal(err)
	}
	first.Timeout = time.Millisecond
	id, err := first.Submit(testEnv("repo.expiring"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if expired := first.CleanupExpired(); expired != 1 {
		t.Fatalf("expired count = %d, want 1", expired)
	}
	if err := firstState.Close(); err != nil {
		t.Fatal(err)
	}

	secondState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondState.Close()
	second, err := NewPersistentQueue(100, secondState.DB(), []byte("expiry-key"))
	if err != nil {
		t.Fatal(err)
	}
	item, err := second.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != StatusExpired {
		t.Fatalf("restored status = %q, want expired", item.Status)
	}
}

func TestPersistentQueueFailsClosedWhenDatabaseIsUnavailable(t *testing.T) {
	store, err := state.OpenSQLite(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	queue, err := NewPersistentQueue(100, store.DB(), []byte("failure-key"))
	if err != nil {
		t.Fatal(err)
	}
	env := testEnv("repo.open_change")
	id, err := queue.Submit(env)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Approve(id, "reviewer", "scope checked"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if queue.ConsumeApprovalForEnvelope(env) {
		t.Fatal("approval was consumed after persistence failed")
	}
	if _, err := queue.Submit(testEnv("repo.other")); err == nil {
		t.Fatal("submission succeeded after persistence failed")
	}
}

func TestPersistentQueueRejectsTamperedItem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	key := []byte("tamper-key")
	firstState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	queue, err := NewPersistentQueue(100, firstState.DB(), key)
	if err != nil {
		t.Fatal(err)
	}
	id, err := queue.Submit(testEnv("repo.write"))
	if err != nil {
		t.Fatal(err)
	}

	var data []byte
	if err := firstState.DB().QueryRow(
		"SELECT item_json FROM approval_items WHERE id = ?", id,
	).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var item ApprovalItem
	if err := json.Unmarshal(data, &item); err != nil {
		t.Fatal(err)
	}
	item.Envelope.Target = "tampered-target"
	tampered, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := firstState.DB().Exec(
		"UPDATE approval_items SET item_json = ? WHERE id = ?", tampered, id,
	); err != nil {
		t.Fatal(err)
	}
	if err := firstState.Close(); err != nil {
		t.Fatal(err)
	}

	secondState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondState.Close()
	if _, err := NewPersistentQueue(100, secondState.DB(), key); err == nil {
		t.Fatal("tampered approval item passed signature check")
	}
}

func TestPersistentQueueRejectsConsumedFlagRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	key := []byte("rollback-key")
	firstState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	queue, err := NewPersistentQueue(100, firstState.DB(), key)
	if err != nil {
		t.Fatal(err)
	}
	env := testEnv("repo.write")
	id, err := queue.Submit(env)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Approve(id, "reviewer", "scope checked"); err != nil {
		t.Fatal(err)
	}
	if !queue.ConsumeApprovalForEnvelope(env) {
		t.Fatal("approved action was not consumed")
	}
	if _, err := firstState.DB().Exec(
		"UPDATE approval_items SET consumed = 0 WHERE id = ?", id,
	); err != nil {
		t.Fatal(err)
	}
	if err := firstState.Close(); err != nil {
		t.Fatal(err)
	}

	secondState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondState.Close()
	if _, err := NewPersistentQueue(100, secondState.DB(), key); err == nil {
		t.Fatal("consumed flag rollback passed signature check")
	}
}

func TestPersistentQueueRejectsDifferentSigningKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	firstState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	queue, err := NewPersistentQueue(100, firstState.DB(), []byte("first-key"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Submit(testEnv("repo.write")); err != nil {
		t.Fatal(err)
	}
	if err := firstState.Close(); err != nil {
		t.Fatal(err)
	}

	secondState, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer secondState.Close()
	if _, err := NewPersistentQueue(100, secondState.DB(), []byte("different-key")); err == nil {
		t.Fatal("approval state accepted a different signing key")
	}
}

func TestPersistentQueueExpiresUnboundMCPApprovals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := state.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("migration-key")
	queue, err := NewPersistentQueue(100, db.DB(), key)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pending", "approved", "consumed", "bound"} {
		env := testEnv(name)
		env.Protocol = envelope.ProtocolMCP
		env.Actor = envelope.ActorInfo{Type: "agent", ID: "mcp-client"}
		if name == "bound" {
			env.Actor = envelope.ActorInfo{Type: "agent", ID: "principal-v1-test", TenantID: "tenant-a", SessionID: "session-v1-test"}
		}
		id, err := queue.Submit(env)
		if err != nil {
			t.Fatal(err)
		}
		if name != "pending" {
			if _, err := queue.Approve(id, "reviewer", "checked"); err != nil {
				t.Fatal(err)
			}
		}
		if name == "consumed" && !queue.ConsumeApprovalForEnvelope(env) {
			t.Fatal("consume failed")
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for restart := 0; restart < 2; restart++ {
		db, err = state.OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		queue, err = NewPersistentQueue(100, db.DB(), key)
		if err != nil {
			t.Fatal(err)
		}
		if len(queue.Pending()) != 0 {
			t.Fatal("legacy pending approval restored")
		}
		for _, name := range []string{"pending", "approved", "consumed", "bound"} {
			item, err := queue.Get("env-" + name)
			if err != nil {
				t.Fatal(err)
			}
			if name == "pending" || name == "approved" {
				if item.Status != StatusExpired {
					t.Fatalf("%s: %s", name, item.Status)
				}
			}
			if name == "consumed" && !item.consumed {
				t.Fatal("consumed state lost")
			}
			if name == "bound" && item.Status != StatusApproved {
				t.Fatal("bound approval lost")
			}
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
