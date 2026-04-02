package pgstore

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/saivedant169/AegisFlow/internal/audit"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) Migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS audit_log_v2 (
		id BIGSERIAL PRIMARY KEY,
		timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		actor VARCHAR(255) NOT NULL,
		actor_role VARCHAR(20) NOT NULL,
		action VARCHAR(100) NOT NULL,
		resource VARCHAR(255) NOT NULL,
		detail TEXT NOT NULL DEFAULT '{}',
		tenant_id VARCHAR(255) NOT NULL DEFAULT '',
		model VARCHAR(255) NOT NULL DEFAULT '',
		previous_hash VARCHAR(64) NOT NULL DEFAULT '',
		entry_hash VARCHAR(64) NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_audit_v2_actor ON audit_log_v2(actor);
	CREATE INDEX IF NOT EXISTS idx_audit_v2_action ON audit_log_v2(action);
	CREATE INDEX IF NOT EXISTS idx_audit_v2_timestamp ON audit_log_v2(timestamp);
	CREATE INDEX IF NOT EXISTS idx_audit_v2_tenant ON audit_log_v2(tenant_id);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *PostgresStore) Insert(entry audit.Entry) error {
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO audit_log_v2 (timestamp, actor, actor_role, action, resource, detail, tenant_id, model, previous_hash, entry_hash)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		entry.Timestamp, entry.Actor, entry.ActorRole, entry.Action, entry.Resource,
		entry.Detail, entry.TenantID, entry.Model, entry.PreviousHash, entry.EntryHash)
	return err
}

func (s *PostgresStore) LastHash() (string, error) {
	var hash string
	err := s.db.QueryRowContext(context.Background(),
		`SELECT entry_hash FROM audit_log_v2 ORDER BY id DESC LIMIT 1`).Scan(&hash)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return hash, err
}

func (s *PostgresStore) Query(filters audit.QueryFilters) ([]audit.Entry, error) {
	query := `SELECT id, timestamp, actor, actor_role, action, resource, detail, tenant_id, model, previous_hash, entry_hash
		FROM audit_log_v2 WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filters.Actor != "" {
		query += fmt.Sprintf(" AND actor = $%d", argIdx)
		args = append(args, filters.Actor)
		argIdx++
	}
	if filters.Action != "" {
		query += fmt.Sprintf(" AND action = $%d", argIdx)
		args = append(args, filters.Action)
		argIdx++
	}
	if filters.TenantID != "" {
		query += fmt.Sprintf(" AND tenant_id = $%d", argIdx)
		args = append(args, filters.TenantID)
		argIdx++
	}
	if !filters.From.IsZero() {
		query += fmt.Sprintf(" AND timestamp >= $%d", argIdx)
		args = append(args, filters.From)
		argIdx++
	}
	if !filters.To.IsZero() {
		query += fmt.Sprintf(" AND timestamp <= $%d", argIdx)
		args = append(args, filters.To)
		argIdx++
	}

	query += " ORDER BY id ASC"
	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIdx)
		args = append(args, filters.Limit)
	}

	rows, err := s.db.QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []audit.Entry
	for rows.Next() {
		var e audit.Entry
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.Actor, &e.ActorRole, &e.Action,
			&e.Resource, &e.Detail, &e.TenantID, &e.Model, &e.PreviousHash, &e.EntryHash); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
