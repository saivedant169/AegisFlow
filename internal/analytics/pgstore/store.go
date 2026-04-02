package pgstore

import (
	"context"
	"database/sql"
	"time"

	"github.com/saivedant169/AegisFlow/internal/analytics"
)

type AnalyticsStore struct {
	db *sql.DB
}

func NewAnalyticsStore(db *sql.DB) *AnalyticsStore {
	return &AnalyticsStore{db: db}
}

func (s *AnalyticsStore) Migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS alerts (
		id VARCHAR(255) PRIMARY KEY,
		severity VARCHAR(10) NOT NULL,
		type VARCHAR(50) NOT NULL,
		dimension VARCHAR(255) NOT NULL,
		metric VARCHAR(50) NOT NULL,
		value DOUBLE PRECISION NOT NULL,
		threshold DOUBLE PRECISION NOT NULL,
		message TEXT NOT NULL,
		state VARCHAR(20) NOT NULL DEFAULT 'active',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		resolved_at TIMESTAMPTZ
	);
	CREATE INDEX IF NOT EXISTS idx_alerts_state ON alerts(state);
	CREATE INDEX IF NOT EXISTS idx_alerts_created ON alerts(created_at);

	CREATE TABLE IF NOT EXISTS metric_aggregates (
		id BIGSERIAL PRIMARY KEY,
		dimension VARCHAR(255) NOT NULL,
		period VARCHAR(10) NOT NULL,
		bucket_start TIMESTAMPTZ NOT NULL,
		request_count BIGINT NOT NULL DEFAULT 0,
		error_count BIGINT NOT NULL DEFAULT 0,
		p50_latency BIGINT NOT NULL DEFAULT 0,
		p95_latency BIGINT NOT NULL DEFAULT 0,
		p99_latency BIGINT NOT NULL DEFAULT 0,
		token_count BIGINT NOT NULL DEFAULT 0,
		estimated_cost DOUBLE PRECISION NOT NULL DEFAULT 0,
		UNIQUE(dimension, period, bucket_start)
	);
	CREATE INDEX IF NOT EXISTS idx_aggregates_dimension ON metric_aggregates(dimension, period, bucket_start);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *AnalyticsStore) SaveAlert(a *analytics.Alert) error {
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO alerts (id, severity, type, dimension, metric, value, threshold, message, state, created_at, resolved_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE SET state=$9, resolved_at=$11`,
		a.ID, a.Severity, a.Type, a.Dimension, a.Metric, a.Value, a.Threshold, a.Message, a.State, a.CreatedAt, a.ResolvedAt)
	return err
}

func (s *AnalyticsStore) FlushAggregates(dim string, period string, buckets []analytics.BucketSummary) error {
	for _, b := range buckets {
		_, err := s.db.ExecContext(context.Background(),
			`INSERT INTO metric_aggregates (dimension, period, bucket_start, request_count, error_count, p50_latency, p95_latency, p99_latency, token_count, estimated_cost)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (dimension, period, bucket_start) DO UPDATE SET
				request_count=$4, error_count=$5, p50_latency=$6, p95_latency=$7, p99_latency=$8, token_count=$9, estimated_cost=$10`,
			dim, period, b.Timestamp, b.Requests, b.Errors, b.P50Latency, b.P95Latency, b.P99Latency, b.Tokens, b.EstimatedCost)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *AnalyticsStore) QueryAggregates(dim, period string, from, to time.Time) ([]analytics.BucketSummary, error) {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT bucket_start, request_count, error_count, p50_latency, p95_latency, p99_latency, token_count, estimated_cost
		FROM metric_aggregates WHERE dimension=$1 AND period=$2 AND bucket_start >= $3 AND bucket_start <= $4
		ORDER BY bucket_start`, dim, period, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []analytics.BucketSummary
	for rows.Next() {
		var b analytics.BucketSummary
		if err := rows.Scan(&b.Timestamp, &b.Requests, &b.Errors, &b.P50Latency, &b.P95Latency, &b.P99Latency, &b.Tokens, &b.EstimatedCost); err != nil {
			return nil, err
		}
		if b.Requests > 0 {
			b.ErrorRate = float64(b.Errors) / float64(b.Requests) * 100
		}
		result = append(result, b)
	}
	return result, rows.Err()
}
