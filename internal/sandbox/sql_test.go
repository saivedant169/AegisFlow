package sandbox

import (
	"testing"

	"github.com/saivedant169/AegisFlow/internal/sqlgate"
)

func TestSQLSandbox_ReadOnly(t *testing.T) {
	s := &SQLSandbox{ReadOnly: true}

	tests := []struct {
		query string
		op    string
		want  bool
	}{
		{"SELECT * FROM users", "select", false},
		{"INSERT INTO users VALUES (1)", "insert", true},
		{"UPDATE users SET name='x'", "update", true},
		{"DELETE FROM users WHERE id=1", "delete", true},
		{"DROP TABLE users", "drop_table", true},
	}

	for _, tt := range tests {
		c := sqlgate.SQLClassification{Operation: tt.op, HasWhereClause: true}
		v := s.Validate(tt.query, c)
		got := v != nil
		if got != tt.want {
			t.Errorf("ReadOnly Validate(%q): got violation=%v, want %v", tt.query, got, tt.want)
		}
		if v != nil && v.Rule != "read_only" {
			t.Errorf("ReadOnly Validate(%q): got rule=%q, want read_only", tt.query, v.Rule)
		}
	}
}

func TestSQLSandbox_BlockDDL(t *testing.T) {
	s := &SQLSandbox{BlockDDL: true}

	tests := []struct {
		op   string
		want bool
	}{
		{"create_table", true},
		{"alter_table", true},
		{"drop_table", true},
		{"drop_database", true},
		{"truncate", true},
		{"select", false},
		{"insert", false},
		{"update", false},
		{"delete", false},
	}

	for _, tt := range tests {
		c := sqlgate.SQLClassification{Operation: tt.op, HasWhereClause: true}
		v := s.Validate("", c)
		got := v != nil
		if got != tt.want {
			t.Errorf("BlockDDL op=%q: got violation=%v, want %v", tt.op, got, tt.want)
		}
	}
}

func TestSQLSandbox_BlockGrant(t *testing.T) {
	s := &SQLSandbox{BlockGrant: true}

	tests := []struct {
		op   string
		want bool
	}{
		{"grant", true},
		{"revoke", true},
		{"select", false},
		{"insert", false},
	}

	for _, tt := range tests {
		c := sqlgate.SQLClassification{Operation: tt.op, HasWhereClause: true}
		v := s.Validate("", c)
		got := v != nil
		if got != tt.want {
			t.Errorf("BlockGrant op=%q: got violation=%v, want %v", tt.op, got, tt.want)
		}
	}
}

func TestSQLSandbox_RequireWhere(t *testing.T) {
	s := &SQLSandbox{RequireWhere: true}

	tests := []struct {
		op       string
		hasWhere bool
		want     bool
	}{
		{"update", false, true},
		{"delete", false, true},
		{"update", true, false},
		{"delete", true, false},
		{"select", false, false}, // SELECT doesn't need WHERE
		{"insert", false, false},
	}

	for _, tt := range tests {
		c := sqlgate.SQLClassification{Operation: tt.op, HasWhereClause: tt.hasWhere}
		v := s.Validate("", c)
		got := v != nil
		if got != tt.want {
			t.Errorf("RequireWhere op=%q hasWhere=%v: got violation=%v, want %v", tt.op, tt.hasWhere, got, tt.want)
		}
	}
}

func TestSQLSandbox_BlockedTables(t *testing.T) {
	s := &SQLSandbox{
		BlockedTables: []string{"secrets", "credentials", "audit_log"},
	}

	tests := []struct {
		table string
		want  bool
	}{
		{"secrets", true},
		{"credentials", true},
		{"audit_log", true},
		{"users", false},
		{"orders", false},
		{"SECRETS", true}, // case insensitive
	}

	for _, tt := range tests {
		c := sqlgate.SQLClassification{Operation: "select", Table: tt.table, HasWhereClause: true}
		v := s.Validate("", c)
		got := v != nil
		if got != tt.want {
			t.Errorf("BlockedTables table=%q: got violation=%v, want %v", tt.table, got, tt.want)
		}
	}
}

func TestSQLSandbox_AllowedSchemas(t *testing.T) {
	s := &SQLSandbox{
		AllowedSchemas: []string{"public", "app"},
	}

	tests := []struct {
		table string
		want  bool
	}{
		{"public.users", false},
		{"app.orders", false},
		{"admin.secrets", true},
		{"users", false}, // no schema prefix, no violation
	}

	for _, tt := range tests {
		c := sqlgate.SQLClassification{Operation: "select", Table: tt.table, HasWhereClause: true}
		v := s.Validate("", c)
		got := v != nil
		if got != tt.want {
			t.Errorf("AllowedSchemas table=%q: got violation=%v, want %v", tt.table, got, tt.want)
		}
	}
}

func TestSQLSandbox_EmptySandbox(t *testing.T) {
	s := &SQLSandbox{}

	c := sqlgate.SQLClassification{Operation: "drop_table", Table: "users"}
	v := s.Validate("DROP TABLE users", c)
	if v != nil {
		t.Errorf("empty sandbox should allow everything, got: %v", v)
	}
}
