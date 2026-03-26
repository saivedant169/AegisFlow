package storage

import (
	"context"
	"database/sql"
	"time"
)

type AuditEntry struct {
	ID          int64     `json:"id"`
	TenantID    string    `json:"tenant_id"`
	Model       string    `json:"model"`
	RequestBody string    `json:"request_body"`
	ResponseBody string   `json:"response_body"`
	StatusCode  int       `json:"status_code"`
	LatencyMs   int64     `json:"latency_ms"`
	PolicyAction string   `json:"policy_action"`
	Cached      bool      `json:"cached"`
	CreatedAt   time.Time `json:"created_at"`
}

func (s *PostgresStore) MigrateAudit() error {
	query := `
	CREATE TABLE IF NOT EXISTS audit_log (
		id BIGSERIAL PRIMARY KEY,
		tenant_id VARCHAR(255) NOT NULL,
		model VARCHAR(255) NOT NULL,
		request_body TEXT NOT NULL DEFAULT '',
		response_body TEXT NOT NULL DEFAULT '',
		status_code INT NOT NULL DEFAULT 200,
		latency_ms BIGINT NOT NULL DEFAULT 0,
		policy_action VARCHAR(50) NOT NULL DEFAULT '',
		cached BOOLEAN NOT NULL DEFAULT false,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_audit_log_tenant ON audit_log(tenant_id);
	CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log(created_at);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *PostgresStore) RecordAudit(ctx context.Context, entry AuditEntry) error {
	query := `
	INSERT INTO audit_log (tenant_id, model, request_body, response_body, status_code, latency_ms, policy_action, cached, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := s.db.ExecContext(ctx, query,
		entry.TenantID, entry.Model, entry.RequestBody, entry.ResponseBody,
		entry.StatusCode, entry.LatencyMs, entry.PolicyAction, entry.Cached,
		entry.CreatedAt,
	)
	return err
}

func (s *PostgresStore) GetAuditLog(ctx context.Context, tenantID string, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	var rows *sql.Rows
	var err error

	if tenantID != "" {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, tenant_id, model, request_body, response_body, status_code, latency_ms, policy_action, cached, created_at
			FROM audit_log WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2`, tenantID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT id, tenant_id, model, request_body, response_body, status_code, latency_ms, policy_action, cached, created_at
			FROM audit_log ORDER BY created_at DESC LIMIT $1`, limit)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.Model, &e.RequestBody, &e.ResponseBody, &e.StatusCode, &e.LatencyMs, &e.PolicyAction, &e.Cached, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
