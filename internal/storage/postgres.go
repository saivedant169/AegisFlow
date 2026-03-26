package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

type UsageEvent struct {
	ID               int64     `json:"id"`
	TenantID         string    `json:"tenant_id"`
	Model            string    `json:"model"`
	Provider         string    `json:"provider"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	EstimatedCostUSD float64   `json:"estimated_cost_usd"`
	Cached           bool      `json:"cached"`
	StatusCode       int       `json:"status_code"`
	LatencyMs        int64     `json:"latency_ms"`
	CreatedAt        time.Time `json:"created_at"`
}

type TenantSummary struct {
	TenantID         string  `json:"tenant_id"`
	TotalRequests    int64   `json:"total_requests"`
	TotalTokens      int64   `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

type ModelSummary struct {
	TenantID         string  `json:"tenant_id"`
	Model            string  `json:"model"`
	Requests         int64   `json:"requests"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(connStr string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	store := &PostgresStore{db: db}
	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return store, nil
}

func (s *PostgresStore) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS usage_events (
		id BIGSERIAL PRIMARY KEY,
		tenant_id VARCHAR(255) NOT NULL,
		model VARCHAR(255) NOT NULL,
		provider VARCHAR(255) NOT NULL DEFAULT '',
		prompt_tokens INT NOT NULL DEFAULT 0,
		completion_tokens INT NOT NULL DEFAULT 0,
		total_tokens INT NOT NULL DEFAULT 0,
		estimated_cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
		cached BOOLEAN NOT NULL DEFAULT false,
		status_code INT NOT NULL DEFAULT 200,
		latency_ms BIGINT NOT NULL DEFAULT 0,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_usage_events_tenant ON usage_events(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_usage_events_model ON usage_events(model);
	CREATE INDEX IF NOT EXISTS idx_usage_events_created ON usage_events(created_at);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *PostgresStore) RecordEvent(ctx context.Context, event UsageEvent) error {
	query := `
	INSERT INTO usage_events (tenant_id, model, provider, prompt_tokens, completion_tokens, total_tokens, estimated_cost_usd, cached, status_code, latency_ms, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`
	_, err := s.db.ExecContext(ctx, query,
		event.TenantID, event.Model, event.Provider,
		event.PromptTokens, event.CompletionTokens, event.TotalTokens,
		event.EstimatedCostUSD, event.Cached, event.StatusCode, event.LatencyMs,
		event.CreatedAt,
	)
	return err
}

func (s *PostgresStore) GetTenantSummaries(ctx context.Context) ([]TenantSummary, error) {
	query := `
	SELECT tenant_id, COUNT(*) as total_requests, COALESCE(SUM(total_tokens), 0) as total_tokens, COALESCE(SUM(estimated_cost_usd), 0) as estimated_cost_usd
	FROM usage_events
	GROUP BY tenant_id
	ORDER BY total_requests DESC
	`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []TenantSummary
	for rows.Next() {
		var s TenantSummary
		if err := rows.Scan(&s.TenantID, &s.TotalRequests, &s.TotalTokens, &s.EstimatedCostUSD); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

func (s *PostgresStore) GetModelSummaries(ctx context.Context, tenantID string) ([]ModelSummary, error) {
	query := `
	SELECT tenant_id, model, COUNT(*) as requests,
		COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0),
		COALESCE(SUM(total_tokens), 0), COALESCE(SUM(estimated_cost_usd), 0)
	FROM usage_events
	WHERE ($1 = '' OR tenant_id = $1)
	GROUP BY tenant_id, model
	ORDER BY requests DESC
	`
	rows, err := s.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []ModelSummary
	for rows.Next() {
		var s ModelSummary
		if err := rows.Scan(&s.TenantID, &s.Model, &s.Requests, &s.PromptTokens, &s.CompletionTokens, &s.TotalTokens, &s.EstimatedCostUSD); err != nil {
			return nil, err
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

func (s *PostgresStore) GetRecentEvents(ctx context.Context, limit int) ([]UsageEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
	SELECT id, tenant_id, model, provider, prompt_tokens, completion_tokens, total_tokens, estimated_cost_usd, cached, status_code, latency_ms, created_at
	FROM usage_events
	ORDER BY created_at DESC
	LIMIT $1
	`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []UsageEvent
	for rows.Next() {
		var e UsageEvent
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Model, &e.Provider, &e.PromptTokens, &e.CompletionTokens, &e.TotalTokens, &e.EstimatedCostUSD, &e.Cached, &e.StatusCode, &e.LatencyMs, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}
