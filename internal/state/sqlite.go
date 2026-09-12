package state

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/saivedant169/AegisFlow/internal/cleanup"
)

// SQLite owns the local runtime state database shared by approval and evidence stores.
type SQLite struct {
	db   *sql.DB
	path string
}

// OpenSQLite opens a local SQLite database with durable, single-writer settings.
func OpenSQLite(path string) (*SQLite, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("sqlite path is required")
	}

	if path != ":memory:" {
		path = filepath.Clean(path)
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create sqlite directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	// One connection keeps PRAGMA settings consistent. Approval and evidence
	// writes are small and already serialized by their owning queues.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	closeOnError := func(err error) (*SQLite, error) {
		cleanup.Close(db)
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return closeOnError(fmt.Errorf("connect sqlite database: %w", err))
	}
	for _, statement := range []string{
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA synchronous = FULL",
	} {
		if _, err := db.Exec(statement); err != nil {
			return closeOnError(fmt.Errorf("configure sqlite database: %w", err))
		}
	}
	if path != ":memory:" {
		if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			return closeOnError(fmt.Errorf("enable sqlite WAL: %w", err))
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return closeOnError(fmt.Errorf("set sqlite permissions: %w", err))
		}
	}

	return &SQLite{db: db, path: path}, nil
}

// DB returns the shared database handle.
func (s *SQLite) DB() *sql.DB {
	return s.db
}

// Path returns the configured database path.
func (s *SQLite) Path() string {
	return s.path
}

// Close flushes and closes the database.
func (s *SQLite) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
