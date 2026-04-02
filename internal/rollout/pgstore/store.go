package pgstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/saivedant169/AegisFlow/internal/rollout"
)

// PostgresStore implements rollout.Store using a PostgreSQL database.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore returns a new PostgresStore backed by the given database connection.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// Migrate creates the rollouts table and indexes if they do not already exist.
func (s *PostgresStore) Migrate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := `
CREATE TABLE IF NOT EXISTS rollouts (
	id                   VARCHAR(36) PRIMARY KEY,
	route_model          VARCHAR(255) NOT NULL,
	baseline_providers   TEXT NOT NULL,
	canary_provider      VARCHAR(255) NOT NULL,
	stages               TEXT NOT NULL,
	current_stage        INTEGER NOT NULL DEFAULT 0,
	current_percentage   INTEGER NOT NULL DEFAULT 0,
	state                VARCHAR(32) NOT NULL DEFAULT 'pending',
	observation_window   BIGINT NOT NULL,
	error_threshold      DOUBLE PRECISION NOT NULL,
	latency_p95_threshold BIGINT NOT NULL,
	stage_started_at     TIMESTAMP NOT NULL,
	created_at           TIMESTAMP NOT NULL,
	updated_at           TIMESTAMP NOT NULL,
	completed_at         TIMESTAMP,
	rollback_reason      TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_rollouts_state ON rollouts (state);
CREATE INDEX IF NOT EXISTS idx_rollouts_route_model ON rollouts (route_model);
`
	_, err := s.db.ExecContext(ctx, query)
	return err
}

// Create inserts a new rollout into the database.
func (s *PostgresStore) Create(r *rollout.Rollout) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stagesJSON, err := json.Marshal(r.Stages)
	if err != nil {
		return fmt.Errorf("marshal stages: %w", err)
	}

	baseline := strings.Join(r.BaselineProviders, ",")
	observationMs := r.ObservationWindow.Milliseconds()

	query := `
INSERT INTO rollouts (
	id, route_model, baseline_providers, canary_provider, stages,
	current_stage, current_percentage, state, observation_window,
	error_threshold, latency_p95_threshold, stage_started_at,
	created_at, updated_at, completed_at, rollback_reason
) VALUES (
	$1, $2, $3, $4, $5,
	$6, $7, $8, $9,
	$10, $11, $12,
	$13, $14, $15, $16
)`

	_, err = s.db.ExecContext(ctx, query,
		r.ID, r.RouteModel, baseline, r.CanaryProvider, string(stagesJSON),
		r.CurrentStage, r.CurrentPercentage, r.State, observationMs,
		r.ErrorThreshold, r.LatencyP95Threshold, r.StageStartedAt,
		r.CreatedAt, r.UpdatedAt, r.CompletedAt, r.RollbackReason,
	)
	return err
}

// Get retrieves a rollout by its ID.
func (s *PostgresStore) Get(id string) (*rollout.Rollout, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
SELECT id, route_model, baseline_providers, canary_provider, stages,
	current_stage, current_percentage, state, observation_window,
	error_threshold, latency_p95_threshold, stage_started_at,
	created_at, updated_at, completed_at, rollback_reason
FROM rollouts WHERE id = $1`

	return s.scanRollout(s.db.QueryRowContext(ctx, query, id))
}

// GetByModel returns the active rollout for a given model (state IN pending, running, paused).
func (s *PostgresStore) GetByModel(model string) (*rollout.Rollout, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
SELECT id, route_model, baseline_providers, canary_provider, stages,
	current_stage, current_percentage, state, observation_window,
	error_threshold, latency_p95_threshold, stage_started_at,
	created_at, updated_at, completed_at, rollback_reason
FROM rollouts
WHERE route_model = $1 AND state IN ($2, $3, $4)
LIMIT 1`

	return s.scanRollout(s.db.QueryRowContext(ctx, query, model, rollout.StatePending, rollout.StateRunning, rollout.StatePaused))
}

// Update persists changes to an existing rollout.
func (s *PostgresStore) Update(r *rollout.Rollout) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stagesJSON, err := json.Marshal(r.Stages)
	if err != nil {
		return fmt.Errorf("marshal stages: %w", err)
	}

	baseline := strings.Join(r.BaselineProviders, ",")
	observationMs := r.ObservationWindow.Milliseconds()

	query := `
UPDATE rollouts SET
	route_model = $1,
	baseline_providers = $2,
	canary_provider = $3,
	stages = $4,
	current_stage = $5,
	current_percentage = $6,
	state = $7,
	observation_window = $8,
	error_threshold = $9,
	latency_p95_threshold = $10,
	stage_started_at = $11,
	updated_at = $12,
	completed_at = $13,
	rollback_reason = $14
WHERE id = $15`

	_, err = s.db.ExecContext(ctx, query,
		r.RouteModel, baseline, r.CanaryProvider, string(stagesJSON),
		r.CurrentStage, r.CurrentPercentage, r.State, observationMs,
		r.ErrorThreshold, r.LatencyP95Threshold, r.StageStartedAt,
		r.UpdatedAt, r.CompletedAt, r.RollbackReason,
		r.ID,
	)
	return err
}

// List returns the most recent 50 rollouts ordered by creation time descending.
func (s *PostgresStore) List() ([]*rollout.Rollout, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
SELECT id, route_model, baseline_providers, canary_provider, stages,
	current_stage, current_percentage, state, observation_window,
	error_threshold, latency_p95_threshold, stage_started_at,
	created_at, updated_at, completed_at, rollback_reason
FROM rollouts
ORDER BY created_at DESC
LIMIT 50`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rollouts []*rollout.Rollout
	for rows.Next() {
		r, err := s.scanRolloutFromRows(rows)
		if err != nil {
			return nil, err
		}
		rollouts = append(rollouts, r)
	}
	return rollouts, rows.Err()
}

// scanRollout scans a single row into a Rollout struct.
func (s *PostgresStore) scanRollout(row *sql.Row) (*rollout.Rollout, error) {
	var r rollout.Rollout
	var baseline, stagesJSON string
	var observationMs int64
	var completedAt sql.NullTime

	err := row.Scan(
		&r.ID, &r.RouteModel, &baseline, &r.CanaryProvider, &stagesJSON,
		&r.CurrentStage, &r.CurrentPercentage, &r.State, &observationMs,
		&r.ErrorThreshold, &r.LatencyP95Threshold, &r.StageStartedAt,
		&r.CreatedAt, &r.UpdatedAt, &completedAt, &r.RollbackReason,
	)
	if err != nil {
		return nil, err
	}

	if baseline != "" {
		r.BaselineProviders = strings.Split(baseline, ",")
	}
	if err := json.Unmarshal([]byte(stagesJSON), &r.Stages); err != nil {
		return nil, fmt.Errorf("unmarshal stages: %w", err)
	}
	r.ObservationWindow = time.Duration(observationMs) * time.Millisecond
	if completedAt.Valid {
		r.CompletedAt = &completedAt.Time
	}

	return &r, nil
}

// scanRolloutFromRows scans a row from sql.Rows into a Rollout struct.
func (s *PostgresStore) scanRolloutFromRows(rows *sql.Rows) (*rollout.Rollout, error) {
	var r rollout.Rollout
	var baseline, stagesJSON string
	var observationMs int64
	var completedAt sql.NullTime

	err := rows.Scan(
		&r.ID, &r.RouteModel, &baseline, &r.CanaryProvider, &stagesJSON,
		&r.CurrentStage, &r.CurrentPercentage, &r.State, &observationMs,
		&r.ErrorThreshold, &r.LatencyP95Threshold, &r.StageStartedAt,
		&r.CreatedAt, &r.UpdatedAt, &completedAt, &r.RollbackReason,
	)
	if err != nil {
		return nil, err
	}

	if baseline != "" {
		r.BaselineProviders = strings.Split(baseline, ",")
	}
	if err := json.Unmarshal([]byte(stagesJSON), &r.Stages); err != nil {
		return nil, fmt.Errorf("unmarshal stages: %w", err)
	}
	r.ObservationWindow = time.Duration(observationMs) * time.Millisecond
	if completedAt.Valid {
		r.CompletedAt = &completedAt.Time
	}

	return &r, nil
}
