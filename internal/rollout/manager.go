package rollout

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/saivedant169/AegisFlow/internal/admin"
)

// Manager wraps a Store and Evaluator to coordinate rollout lifecycle operations
// and background health evaluation.
type Manager struct {
	store     Store
	evaluator *Evaluator
	stopCh    chan struct{}
}

// NewManager creates a new Manager, running store migration and initialising the evaluator.
func NewManager(store Store, reqLog *admin.RequestLog) (*Manager, error) {
	if err := store.Migrate(); err != nil {
		return nil, fmt.Errorf("store migrate: %w", err)
	}
	return &Manager{
		store:     store,
		evaluator: NewEvaluator(reqLog),
		stopCh:    make(chan struct{}),
	}, nil
}

// Start launches the background evaluation loop.
func (m *Manager) Start() {
	go m.evaluationLoop()
}

// Stop signals the background evaluation loop to exit.
func (m *Manager) Stop() {
	close(m.stopCh)
}

func (m *Manager) evaluationLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.evaluateAll()
		}
	}
}

func (m *Manager) evaluateAll() {
	rollouts, err := m.store.List()
	if err != nil {
		return
	}
	for _, r := range rollouts {
		if r.State != StateRunning {
			continue
		}
		decision, reason, metrics := m.evaluator.Evaluate(r)
		switch decision {
		case "promote":
			m.promote(r, metrics)
		case "rollback":
			m.doRollback(r, reason)
		}
	}
}

func (m *Manager) promote(r *Rollout, metrics RolloutMetrics) {
	now := time.Now()
	if r.CurrentStage >= len(r.Stages)-1 {
		// Last stage — mark completed.
		r.State = StateCompleted
		r.CompletedAt = &now
	} else {
		r.CurrentStage++
		r.CurrentPercentage = r.Stages[r.CurrentStage]
		r.StageStartedAt = now
	}
	r.UpdatedAt = now
	_ = m.store.Update(r)
}

func (m *Manager) doRollback(r *Rollout, reason string) {
	now := time.Now()
	r.State = StateRolledBack
	r.CurrentPercentage = 0
	r.RollbackReason = reason
	r.UpdatedAt = now
	r.CompletedAt = &now
	_ = m.store.Update(r)
}

// CreateRollout creates a new canary rollout for the given model. It returns an
// error if there is already an active rollout for the same model.
func (m *Manager) CreateRollout(
	routeModel string,
	baselineProviders []string,
	canaryProvider string,
	stages []int,
	observationWindow time.Duration,
	errorThreshold float64,
	latencyP95Threshold int64,
) (*Rollout, error) {
	// Check for existing active rollout on the same model.
	existing, err := m.store.GetByModel(routeModel)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("active rollout %s already exists for model %s", existing.ID, routeModel)
	}

	now := time.Now()
	r := &Rollout{
		ID:                  fmt.Sprintf("r-%d", now.UnixNano()),
		RouteModel:          routeModel,
		BaselineProviders:   baselineProviders,
		CanaryProvider:      canaryProvider,
		Stages:              stages,
		CurrentStage:        0,
		CurrentPercentage:   stages[0],
		State:               StateRunning,
		ObservationWindow:   observationWindow,
		ErrorThreshold:      errorThreshold,
		LatencyP95Threshold: latencyP95Threshold,
		StageStartedAt:      now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := m.store.Create(r); err != nil {
		return nil, fmt.Errorf("create rollout: %w", err)
	}
	return r, nil
}

// ActiveRollout returns the running rollout for the given model, or nil if none exists.
func (m *Manager) ActiveRollout(model string) *Rollout {
	r, err := m.store.GetByModel(model)
	if err != nil || r == nil {
		return nil
	}
	if r.State != StateRunning {
		return nil
	}
	return r
}

// GetRollout retrieves a rollout by ID.
func (m *Manager) GetRollout(id string) (*Rollout, error) {
	r, err := m.store.Get(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("rollout %s not found", id)
		}
		return nil, err
	}
	return r, nil
}

// ListRollouts returns all rollouts.
func (m *Manager) ListRollouts() ([]*Rollout, error) {
	return m.store.List()
}

// PauseRollout transitions a rollout from running to paused.
func (m *Manager) PauseRollout(id string) error {
	r, err := m.store.Get(id)
	if err != nil {
		return fmt.Errorf("get rollout: %w", err)
	}
	if r.State != StateRunning {
		return fmt.Errorf("cannot pause rollout in state %s", r.State)
	}
	r.State = StatePaused
	r.UpdatedAt = time.Now()
	return m.store.Update(r)
}

// ResumeRollout transitions a rollout from paused to running and resets StageStartedAt.
func (m *Manager) ResumeRollout(id string) error {
	r, err := m.store.Get(id)
	if err != nil {
		return fmt.Errorf("get rollout: %w", err)
	}
	if r.State != StatePaused {
		return fmt.Errorf("cannot resume rollout in state %s", r.State)
	}
	now := time.Now()
	r.State = StateRunning
	r.StageStartedAt = now
	r.UpdatedAt = now
	return m.store.Update(r)
}

// RollbackRollout manually rolls back a rollout from running or paused state.
func (m *Manager) RollbackRollout(id string) error {
	r, err := m.store.Get(id)
	if err != nil {
		return fmt.Errorf("get rollout: %w", err)
	}
	if r.State != StateRunning && r.State != StatePaused {
		return fmt.Errorf("cannot rollback rollout in state %s", r.State)
	}
	now := time.Now()
	r.State = StateRolledBack
	r.CurrentPercentage = 0
	r.RollbackReason = "manual rollback"
	r.UpdatedAt = now
	r.CompletedAt = &now
	return m.store.Update(r)
}

// GetMetrics calls the evaluator and returns the current rollout metrics.
func (m *Manager) GetMetrics(r *Rollout) RolloutMetrics {
	_, _, metrics := m.evaluator.Evaluate(r)
	return metrics
}
