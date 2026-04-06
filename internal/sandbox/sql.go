package sandbox

import (
	"strings"

	"github.com/saivedant169/AegisFlow/internal/sqlgate"
)

// SQLSandbox enforces constraints on SQL query execution.
type SQLSandbox struct {
	ReadOnly       bool     `yaml:"read_only" json:"read_only"`
	AllowedSchemas []string `yaml:"allowed_schemas" json:"allowed_schemas"`
	BlockedTables  []string `yaml:"blocked_tables" json:"blocked_tables"`
	MaxRowsReturn  int      `yaml:"max_rows_return" json:"max_rows_return"`
	BlockDDL       bool     `yaml:"block_ddl" json:"block_ddl"`
	BlockGrant     bool     `yaml:"block_grant" json:"block_grant"`
	RequireWhere   bool     `yaml:"require_where" json:"require_where"`
}

// Validate checks a SQL query against sandbox constraints and returns the
// first violation found, or nil if the query is allowed.
func (s *SQLSandbox) Validate(query string, classification sqlgate.SQLClassification) *SandboxViolation {
	op := classification.Operation

	// Read-only mode: block anything that is not a SELECT.
	if s.ReadOnly && op != "select" {
		return &SandboxViolation{
			SandboxType: "sql",
			Rule:        "read_only",
			Message:     "sandbox is read-only; operation not allowed: " + op,
			Severity:    "block",
		}
	}

	// Block DDL operations.
	if s.BlockDDL && isDDL(op) {
		return &SandboxViolation{
			SandboxType: "sql",
			Rule:        "block_ddl",
			Message:     "DDL operations are blocked: " + op,
			Severity:    "block",
		}
	}

	// Block GRANT/REVOKE.
	if s.BlockGrant && isGrant(op) {
		return &SandboxViolation{
			SandboxType: "sql",
			Rule:        "block_grant",
			Message:     "GRANT/REVOKE operations are blocked: " + op,
			Severity:    "block",
		}
	}

	// Require WHERE clause on UPDATE/DELETE.
	if s.RequireWhere && isModify(op) && !classification.HasWhereClause {
		return &SandboxViolation{
			SandboxType: "sql",
			Rule:        "require_where",
			Message:     op + " without WHERE clause is blocked",
			Severity:    "block",
		}
	}

	// Check blocked tables.
	if len(s.BlockedTables) > 0 && classification.Table != "" {
		tableLower := strings.ToLower(classification.Table)
		for _, blocked := range s.BlockedTables {
			if strings.ToLower(blocked) == tableLower {
				return &SandboxViolation{
					SandboxType: "sql",
					Rule:        "blocked_table",
					Message:     "access to table is blocked: " + classification.Table,
					Severity:    "block",
				}
			}
		}
	}

	// Check allowed schemas (table must be schema-qualified: schema.table).
	if len(s.AllowedSchemas) > 0 && classification.Table != "" {
		schema := extractSchema(classification.Table)
		if schema != "" {
			allowed := false
			for _, s := range s.AllowedSchemas {
				if strings.EqualFold(s, schema) {
					allowed = true
					break
				}
			}
			if !allowed {
				return &SandboxViolation{
					SandboxType: "sql",
					Rule:        "allowed_schema",
					Message:     "schema not in allowlist: " + schema,
					Severity:    "block",
				}
			}
		}
	}

	return nil
}

func isDDL(op string) bool {
	switch op {
	case "create_table", "alter_table", "drop_table", "drop_database", "truncate":
		return true
	}
	return false
}

func isGrant(op string) bool {
	return op == "grant" || op == "revoke"
}

func isModify(op string) bool {
	return op == "update" || op == "delete"
}

// extractSchema extracts the schema portion from a schema.table reference.
func extractSchema(table string) string {
	if idx := strings.Index(table, "."); idx > 0 {
		return table[:idx]
	}
	return ""
}
