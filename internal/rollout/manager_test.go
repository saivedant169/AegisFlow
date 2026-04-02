package rollout

import (
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/admin"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	store := NewMemoryStore()
	reqLog := admin.NewRequestLog(100)
	mgr, err := NewManager(store, reqLog)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr
}

func TestCreateRollout(t *testing.T) {
	mgr := newTestManager(t)
	r, err := mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 25, 50, 100}, 30*time.Second, 5.0, 500)
	if err != nil {
		t.Fatalf("CreateRollout: %v", err)
	}
	if r.State != StateRunning {
		t.Errorf("expected state %s, got %s", StateRunning, r.State)
	}
	if r.CurrentPercentage != 10 {
		t.Errorf("expected percentage 10, got %d", r.CurrentPercentage)
	}
	if r.CurrentStage != 0 {
		t.Errorf("expected stage 0, got %d", r.CurrentStage)
	}
}

func TestDuplicateRolloutRejected(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 50, 100}, 30*time.Second, 5.0, 500)
	if err != nil {
		t.Fatalf("first CreateRollout: %v", err)
	}
	_, err = mgr.CreateRollout("gpt-4", []string{"openai"}, "bedrock", []int{10, 50, 100}, 30*time.Second, 5.0, 500)
	if err == nil {
		t.Fatal("expected error for duplicate rollout on same model, got nil")
	}
}

func TestPauseResumeRollout(t *testing.T) {
	mgr := newTestManager(t)
	r, _ := mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 50, 100}, 30*time.Second, 5.0, 500)

	if err := mgr.PauseRollout(r.ID); err != nil {
		t.Fatalf("PauseRollout: %v", err)
	}
	paused, _ := mgr.GetRollout(r.ID)
	if paused.State != StatePaused {
		t.Errorf("expected state %s, got %s", StatePaused, paused.State)
	}

	if err := mgr.ResumeRollout(r.ID); err != nil {
		t.Fatalf("ResumeRollout: %v", err)
	}
	resumed, _ := mgr.GetRollout(r.ID)
	if resumed.State != StateRunning {
		t.Errorf("expected state %s, got %s", StateRunning, resumed.State)
	}
	if !resumed.StageStartedAt.After(r.StageStartedAt) {
		t.Error("expected StageStartedAt to be reset after resume")
	}
}

func TestManualRollback(t *testing.T) {
	mgr := newTestManager(t)
	r, _ := mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 50, 100}, 30*time.Second, 5.0, 500)

	if err := mgr.RollbackRollout(r.ID); err != nil {
		t.Fatalf("RollbackRollout: %v", err)
	}
	rolled, _ := mgr.GetRollout(r.ID)
	if rolled.State != StateRolledBack {
		t.Errorf("expected state %s, got %s", StateRolledBack, rolled.State)
	}
	if rolled.CurrentPercentage != 0 {
		t.Errorf("expected percentage 0, got %d", rolled.CurrentPercentage)
	}
}

func TestActiveRollout(t *testing.T) {
	mgr := newTestManager(t)

	// No active rollout yet.
	if got := mgr.ActiveRollout("gpt-4"); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}

	r, _ := mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 50, 100}, 30*time.Second, 5.0, 500)
	active := mgr.ActiveRollout("gpt-4")
	if active == nil {
		t.Fatal("expected active rollout, got nil")
	}
	if active.ID != r.ID {
		t.Errorf("expected ID %s, got %s", r.ID, active.ID)
	}
}

func TestPromoteToFinalStageCompletesRollout(t *testing.T) {
	mgr := newTestManager(t)
	r, err := mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 50, 100}, 30*time.Second, 5.0, 500)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate promote through stages by directly calling promote on the manager.
	// Stage 0 (10%) -> promote to stage 1 (50%)
	fetched, _ := mgr.store.Get(r.ID)
	mgr.promote(fetched, RolloutMetrics{})
	fetched, _ = mgr.store.Get(r.ID)
	if fetched.CurrentStage != 1 || fetched.CurrentPercentage != 50 {
		t.Fatalf("expected stage 1 at 50%%, got stage %d at %d%%", fetched.CurrentStage, fetched.CurrentPercentage)
	}
	if fetched.State != StateRunning {
		t.Fatalf("expected running after intermediate promote, got %s", fetched.State)
	}

	// Stage 1 (50%) -> promote to stage 2 (100%)
	mgr.promote(fetched, RolloutMetrics{})
	fetched, _ = mgr.store.Get(r.ID)
	if fetched.CurrentStage != 2 || fetched.CurrentPercentage != 100 {
		t.Fatalf("expected stage 2 at 100%%, got stage %d at %d%%", fetched.CurrentStage, fetched.CurrentPercentage)
	}

	// Stage 2 (100%, last stage) -> promote completes
	mgr.promote(fetched, RolloutMetrics{})
	fetched, _ = mgr.store.Get(r.ID)
	if fetched.State != StateCompleted {
		t.Fatalf("expected completed after final promote, got %s", fetched.State)
	}
	if fetched.CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set")
	}
}

func TestDoRollbackSetsPercentageToZero(t *testing.T) {
	mgr := newTestManager(t)
	r, _ := mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 50, 100}, 30*time.Second, 5.0, 500)

	fetched, _ := mgr.store.Get(r.ID)
	mgr.doRollback(fetched, "test reason")

	fetched, _ = mgr.store.Get(r.ID)
	if fetched.State != StateRolledBack {
		t.Errorf("expected state rolled_back, got %s", fetched.State)
	}
	if fetched.CurrentPercentage != 0 {
		t.Errorf("expected percentage 0, got %d", fetched.CurrentPercentage)
	}
	if fetched.RollbackReason != "test reason" {
		t.Errorf("expected reason 'test reason', got '%s'", fetched.RollbackReason)
	}
	if fetched.CompletedAt == nil {
		t.Error("expected CompletedAt to be set after rollback")
	}
}

func TestEvaluateAllSkipsNonRunningRollouts(t *testing.T) {
	mgr := newTestManager(t)
	r, _ := mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 50, 100}, 30*time.Second, 5.0, 500)

	// Pause the rollout so it's not running.
	_ = mgr.PauseRollout(r.ID)

	// evaluateAll should skip this rollout (no panic, no state change).
	mgr.evaluateAll()

	fetched, _ := mgr.GetRollout(r.ID)
	if fetched.State != StatePaused {
		t.Errorf("expected state paused (unchanged), got %s", fetched.State)
	}
}

func TestGetRolloutNonexistent(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.GetRollout("nonexistent-id")
	if err == nil {
		t.Fatal("expected error for nonexistent rollout")
	}
}

func TestListRolloutsOrdering(t *testing.T) {
	mgr := newTestManager(t)

	// Create rollouts with different models so they don't conflict.
	r1, _ := mgr.CreateRollout("model-a", []string{"p1"}, "canary", []int{10, 100}, 30*time.Second, 5.0, 500)
	time.Sleep(time.Millisecond) // ensure different CreatedAt
	r2, _ := mgr.CreateRollout("model-b", []string{"p1"}, "canary", []int{10, 100}, 30*time.Second, 5.0, 500)

	list, err := mgr.ListRollouts()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 rollouts, got %d", len(list))
	}
	// Newest first
	if list[0].ID != r2.ID {
		t.Errorf("expected newest rollout first (ID %s), got %s", r2.ID, list[0].ID)
	}
	if list[1].ID != r1.ID {
		t.Errorf("expected oldest rollout second (ID %s), got %s", r1.ID, list[1].ID)
	}
}

func TestMemoryStoreCreateDuplicateReturnsError(t *testing.T) {
	store := NewMemoryStore()
	r := &Rollout{ID: "r-dup", State: StateRunning}
	if err := store.Create(r); err != nil {
		t.Fatal(err)
	}
	err := store.Create(r)
	if err == nil {
		t.Fatal("expected error for duplicate ID")
	}
}

func TestMemoryStoreGetNonexistentReturnsError(t *testing.T) {
	store := NewMemoryStore()
	_, err := store.Get("no-such-id")
	if err == nil {
		t.Fatal("expected error for nonexistent ID")
	}
}

func TestMemoryStoreUpdateNonexistentReturnsError(t *testing.T) {
	store := NewMemoryStore()
	r := &Rollout{ID: "no-such-id", State: StateRunning}
	err := store.Update(r)
	if err == nil {
		t.Fatal("expected error for updating nonexistent ID")
	}
}

func TestInvalidStateTransitions(t *testing.T) {
	mgr := newTestManager(t)
	r, _ := mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 50, 100}, 30*time.Second, 5.0, 500)

	// Cannot resume a running rollout.
	if err := mgr.ResumeRollout(r.ID); err == nil {
		t.Error("expected error resuming a running rollout")
	}

	// Rollback the rollout.
	_ = mgr.RollbackRollout(r.ID)

	// Cannot pause a rolled-back rollout.
	if err := mgr.PauseRollout(r.ID); err == nil {
		t.Error("expected error pausing a rolled_back rollout")
	}

	// Cannot resume a rolled-back rollout.
	if err := mgr.ResumeRollout(r.ID); err == nil {
		t.Error("expected error resuming a rolled_back rollout")
	}

	// Cannot rollback a rolled-back rollout.
	if err := mgr.RollbackRollout(r.ID); err == nil {
		t.Error("expected error rolling back a rolled_back rollout")
	}
}

func TestGetMetrics(t *testing.T) {
	mgr := newTestManager(t)
	r, _ := mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 50, 100}, 30*time.Second, 5.0, 500)

	metrics := mgr.GetMetrics(r)
	// With no request log entries, metrics should be zero-valued.
	if metrics.Baseline.Requests != 0 {
		t.Errorf("expected 0 baseline requests, got %d", metrics.Baseline.Requests)
	}
	if metrics.Canary.Requests != 0 {
		t.Errorf("expected 0 canary requests, got %d", metrics.Canary.Requests)
	}
}

func TestStartStop(t *testing.T) {
	mgr := newTestManager(t)
	mgr.Start()
	// Just verify it doesn't panic and can be stopped.
	mgr.Stop()
}

func TestAdminAdapterListRollouts(t *testing.T) {
	mgr := newTestManager(t)
	adapter := NewAdminAdapter(mgr)

	_, _ = mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 100}, 30*time.Second, 5.0, 500)

	result, err := adapter.ListRollouts()
	if err != nil {
		t.Fatal(err)
	}
	rollouts, ok := result.([]*Rollout)
	if !ok {
		t.Fatal("expected []*Rollout from ListRollouts")
	}
	if len(rollouts) != 1 {
		t.Fatalf("expected 1 rollout, got %d", len(rollouts))
	}
}

func TestAdminAdapterCreateRollout(t *testing.T) {
	mgr := newTestManager(t)
	adapter := NewAdminAdapter(mgr)

	result, err := adapter.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 100}, 30*time.Second, 5.0, 500)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := result.(*Rollout)
	if !ok {
		t.Fatal("expected *Rollout from CreateRollout")
	}
	if r.State != StateRunning {
		t.Errorf("expected running state, got %s", r.State)
	}
}

func TestAdminAdapterGetRolloutWithMetrics(t *testing.T) {
	mgr := newTestManager(t)
	adapter := NewAdminAdapter(mgr)

	r, _ := mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 100}, 30*time.Second, 5.0, 500)

	result, err := adapter.GetRolloutWithMetrics(r.ID)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map from GetRolloutWithMetrics")
	}
	if m["rollout"] == nil {
		t.Error("expected rollout in result")
	}
	if m["metrics"] == nil {
		t.Error("expected metrics in result")
	}

	// Test nonexistent ID.
	_, err = adapter.GetRolloutWithMetrics("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent rollout")
	}
}

func TestAdminAdapterPauseResumeRollback(t *testing.T) {
	mgr := newTestManager(t)
	adapter := NewAdminAdapter(mgr)

	r, _ := mgr.CreateRollout("gpt-4", []string{"openai"}, "anthropic", []int{10, 100}, 30*time.Second, 5.0, 500)

	if err := adapter.PauseRollout(r.ID); err != nil {
		t.Fatal(err)
	}
	if err := adapter.ResumeRollout(r.ID); err != nil {
		t.Fatal(err)
	}
	if err := adapter.RollbackRollout(r.ID); err != nil {
		t.Fatal(err)
	}
}
