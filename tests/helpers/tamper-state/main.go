package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	_ "modernc.org/sqlite"

	"github.com/saivedant169/AegisFlow/internal/cleanup"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: tamper-state <approval|evidence> <sqlite-path>")
		os.Exit(2)
	}
	db, err := sql.Open("sqlite", os.Args[2])
	if err != nil {
		fail(err)
	}
	defer cleanup.Close(db)

	switch os.Args[1] {
	case "approval":
		tamperApproval(db)
	case "evidence":
		tamperEvidence(db)
	default:
		fail(fmt.Errorf("unknown state type %q", os.Args[1]))
	}
}

func tamperApproval(db *sql.DB) {
	var id string
	var data []byte
	err := db.QueryRow(`
		SELECT id, item_json
		FROM approval_items
		ORDER BY submitted_at_ns
		LIMIT 1
	`).Scan(&id, &data)
	if err != nil {
		fail(err)
	}
	tampered := changeEnvelopeTarget(data)
	if _, err := db.Exec("UPDATE approval_items SET item_json = ? WHERE id = ?", tampered, id); err != nil {
		fail(err)
	}
}

func tamperEvidence(db *sql.DB) {
	var sessionID string
	var index int
	var data []byte
	err := db.QueryRow(`
		SELECT session_id, record_index, record_json
		FROM evidence_records
		ORDER BY session_id, record_index
		LIMIT 1
	`).Scan(&sessionID, &index, &data)
	if err != nil {
		fail(err)
	}
	tampered := changeEnvelopeTarget(data)
	if _, err := db.Exec(
		"UPDATE evidence_records SET record_json = ? WHERE session_id = ? AND record_index = ?",
		tampered, sessionID, index,
	); err != nil {
		fail(err)
	}
}

func changeEnvelopeTarget(data []byte) []byte {
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		fail(err)
	}
	envelope, ok := record["envelope"].(map[string]any)
	if !ok {
		fail(fmt.Errorf("record has no envelope object"))
	}
	envelope["target"] = "tampered-target"
	tampered, err := json.Marshal(record)
	if err != nil {
		fail(err)
	}
	return tampered
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
