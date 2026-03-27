package rollout

import (
	"testing"
	"time"

	"github.com/aegisflow/aegisflow/internal/admin"
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
