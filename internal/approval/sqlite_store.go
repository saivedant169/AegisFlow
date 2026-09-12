package approval

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/saivedant169/AegisFlow/internal/cleanup"
)

type approvalStore interface {
	Load(historyLimit int) ([]*ApprovalItem, []*ApprovalItem, error)
	Save(item *ApprovalItem) error
	Consume(item *ApprovalItem) (bool, error)
	PruneHistory(limit int) error
}

type sqliteApprovalStore struct {
	db  *sql.DB
	key []byte
}

func newSQLiteApprovalStore(db *sql.DB, key []byte) (*sqliteApprovalStore, error) {
	if db == nil {
		return nil, fmt.Errorf("approval sqlite database is required")
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("approval state signing key is required")
	}
	store := &sqliteApprovalStore{db: db, key: append([]byte(nil), key...)}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS approval_items (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			submitted_at_ns INTEGER NOT NULL,
			reviewed_at_ns INTEGER NOT NULL DEFAULT 0,
			consumed INTEGER NOT NULL DEFAULT 0 CHECK (consumed IN (0, 1)),
			signature TEXT NOT NULL,
			item_json BLOB NOT NULL
		);
		CREATE INDEX IF NOT EXISTS approval_items_status_time
			ON approval_items (status, reviewed_at_ns, submitted_at_ns);
	`); err != nil {
		return nil, fmt.Errorf("migrate approval state: %w", err)
	}
	return store, nil
}

func (s *sqliteApprovalStore) Load(historyLimit int) ([]*ApprovalItem, []*ApprovalItem, error) {
	pending, err := s.loadItems(
		"SELECT id, status, submitted_at_ns, reviewed_at_ns, item_json, consumed, signature FROM approval_items WHERE status = ? ORDER BY submitted_at_ns",
		StatusPending,
	)
	if err != nil {
		return nil, nil, err
	}
	history, err := s.loadItems(
		"SELECT id, status, submitted_at_ns, reviewed_at_ns, item_json, consumed, signature FROM approval_items WHERE status <> ? ORDER BY reviewed_at_ns DESC, submitted_at_ns DESC LIMIT ?",
		StatusPending, historyLimit,
	)
	if err != nil {
		return nil, nil, err
	}
	for left, right := 0, len(history)-1; left < right; left, right = left+1, right-1 {
		history[left], history[right] = history[right], history[left]
	}
	return pending, history, nil
}

func (s *sqliteApprovalStore) loadItems(query string, args ...any) ([]*ApprovalItem, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query approval items: %w", err)
	}
	defer cleanup.Close(rows)

	items := make([]*ApprovalItem, 0)
	for rows.Next() {
		var id, status string
		var submittedAt, reviewedAt int64
		var data []byte
		var consumed int
		var signature string
		if err := rows.Scan(&id, &status, &submittedAt, &reviewedAt, &data, &consumed, &signature); err != nil {
			return nil, fmt.Errorf("scan approval item: %w", err)
		}
		if !hmac.Equal([]byte(signature), []byte(s.sign(data, consumed))) {
			return nil, fmt.Errorf("approval item signature mismatch")
		}
		var item ApprovalItem
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("decode approval item: %w", err)
		}
		if item.ID != id || item.Status != status || item.SubmittedAt.UnixNano() != submittedAt {
			return nil, fmt.Errorf("approval item index fields do not match signed content")
		}
		if item.ReviewedAt == nil && reviewedAt != 0 {
			return nil, fmt.Errorf("approval item review time does not match signed content")
		}
		if item.ReviewedAt != nil && item.ReviewedAt.UnixNano() != reviewedAt {
			return nil, fmt.Errorf("approval item review time does not match signed content")
		}
		item.consumed = consumed == 1
		items = append(items, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read approval items: %w", err)
	}
	return items, nil
}

func (s *sqliteApprovalStore) Save(item *ApprovalItem) error {
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("encode approval item: %w", err)
	}
	reviewedAt := int64(0)
	if item.ReviewedAt != nil {
		reviewedAt = item.ReviewedAt.UnixNano()
	}
	consumed := 0
	if item.consumed {
		consumed = 1
	}
	signature := s.sign(data, consumed)
	if _, err := s.db.Exec(`
		INSERT INTO approval_items
			(id, status, submitted_at_ns, reviewed_at_ns, consumed, signature, item_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			submitted_at_ns = excluded.submitted_at_ns,
			reviewed_at_ns = excluded.reviewed_at_ns,
			consumed = excluded.consumed,
			signature = excluded.signature,
			item_json = excluded.item_json
	`, item.ID, item.Status, item.SubmittedAt.UnixNano(), reviewedAt, consumed, signature, data); err != nil {
		return fmt.Errorf("save approval item: %w", err)
	}
	return nil
}

func (s *sqliteApprovalStore) Consume(item *ApprovalItem) (bool, error) {
	data, err := json.Marshal(item)
	if err != nil {
		return false, fmt.Errorf("encode consumed approval item: %w", err)
	}
	currentSignature := s.sign(data, 0)
	consumedSignature := s.sign(data, 1)
	result, err := s.db.Exec(`
		UPDATE approval_items
		SET consumed = 1, signature = ?
		WHERE id = ? AND status = ? AND consumed = 0 AND signature = ?
	`, consumedSignature, item.ID, StatusApproved, currentSignature)
	if err != nil {
		return false, fmt.Errorf("consume approval item: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read consumed approval count: %w", err)
	}
	return count == 1, nil
}

func (s *sqliteApprovalStore) sign(data []byte, consumed int) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte("aegisflow-approval-state-v1\x00"))
	mac.Write(data)
	mac.Write([]byte{byte(consumed)})
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *sqliteApprovalStore) PruneHistory(limit int) error {
	if limit <= 0 {
		return nil
	}
	if _, err := s.db.Exec(`
		DELETE FROM approval_items
		WHERE id IN (
			SELECT id FROM approval_items
			WHERE status <> ?
			ORDER BY reviewed_at_ns DESC, submitted_at_ns DESC
			LIMIT -1 OFFSET ?
		)
	`, StatusPending, limit); err != nil {
		return fmt.Errorf("prune approval history: %w", err)
	}
	return nil
}
