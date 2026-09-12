package evidence

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/saivedant169/AegisFlow/internal/cleanup"
)

type sqliteRecordStore struct {
	db  *sql.DB
	key []byte
}

func newSQLiteRecordStore(db *sql.DB, key []byte) (*sqliteRecordStore, error) {
	if db == nil {
		return nil, fmt.Errorf("evidence sqlite database is required")
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("evidence state signing key is required")
	}
	store := &sqliteRecordStore{db: db, key: append([]byte(nil), key...)}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS evidence_records (
			session_id TEXT NOT NULL,
			record_index INTEGER NOT NULL CHECK (record_index >= 0),
			record_signature TEXT NOT NULL,
			record_json BLOB NOT NULL,
			PRIMARY KEY (session_id, record_index)
		);
		CREATE INDEX IF NOT EXISTS evidence_records_session
			ON evidence_records (session_id, record_index);
	`); err != nil {
		return nil, fmt.Errorf("migrate evidence state: %w", err)
	}
	return store, nil
}

func (s *sqliteRecordStore) Append(sessionID string, record Record) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode record: %w", err)
	}
	signature := s.sign(sessionID, record.Index, data)
	if _, err := s.db.Exec(
		"INSERT INTO evidence_records (session_id, record_index, record_signature, record_json) VALUES (?, ?, ?, ?)",
		sessionID, record.Index, signature, data,
	); err != nil {
		return fmt.Errorf("insert record: %w", err)
	}
	return nil
}

func (s *sqliteRecordStore) LoadSession(sessionID string) ([]Record, error) {
	rows, err := s.db.Query(
		"SELECT record_index, record_signature, record_json FROM evidence_records WHERE session_id = ? ORDER BY record_index",
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}
	defer cleanup.Close(rows)

	records := make([]Record, 0)
	for rows.Next() {
		var index int
		var signature string
		var data []byte
		if err := rows.Scan(&index, &signature, &data); err != nil {
			return nil, fmt.Errorf("scan record: %w", err)
		}
		if !hmac.Equal([]byte(signature), []byte(s.sign(sessionID, index, data))) {
			return nil, fmt.Errorf("evidence record storage signature mismatch")
		}
		var record Record
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("decode record %d: %w", index, err)
		}
		if record.Index != index {
			return nil, fmt.Errorf("stored record index %d does not match row index %d", record.Index, index)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read session records: %w", err)
	}
	return records, nil
}

func (s *sqliteRecordStore) sign(sessionID string, index int, data []byte) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte("aegisflow-evidence-state-v1\x00"))
	var field [8]byte
	binary.BigEndian.PutUint64(field[:], uint64(len(sessionID)))
	mac.Write(field[:])
	mac.Write([]byte(sessionID))
	binary.BigEndian.PutUint64(field[:], uint64(index))
	mac.Write(field[:])
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *sqliteRecordStore) ListSessionIDs() ([]string, error) {
	rows, err := s.db.Query("SELECT DISTINCT session_id FROM evidence_records ORDER BY session_id")
	if err != nil {
		return nil, fmt.Errorf("list evidence sessions: %w", err)
	}
	defer cleanup.Close(rows)

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan session id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read evidence sessions: %w", err)
	}
	return ids, nil
}
