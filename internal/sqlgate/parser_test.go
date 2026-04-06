package sqlgate

import (
	"testing"
)

func TestClassifySelect(t *testing.T) {
	c := ClassifySQL("SELECT id, name FROM users WHERE id = 1")
	if c.Operation != "select" {
		t.Fatalf("expected select, got %s", c.Operation)
	}
	if c.Table != "users" {
		t.Fatalf("expected table users, got %s", c.Table)
	}
	if !c.HasWhereClause {
		t.Fatal("expected HasWhereClause to be true")
	}
	if c.IsDangerous {
		t.Fatal("expected IsDangerous to be false")
	}
}

func TestClassifyInsert(t *testing.T) {
	c := ClassifySQL("INSERT INTO orders (product, qty) VALUES ('widget', 5)")
	if c.Operation != "insert" {
		t.Fatalf("expected insert, got %s", c.Operation)
	}
	if c.Table != "orders" {
		t.Fatalf("expected table orders, got %s", c.Table)
	}
	if c.IsDangerous {
		t.Fatal("expected IsDangerous to be false")
	}
}

func TestClassifyUpdate(t *testing.T) {
	c := ClassifySQL("UPDATE users SET name = 'alice' WHERE id = 1")
	if c.Operation != "update" {
		t.Fatalf("expected update, got %s", c.Operation)
	}
	if c.Table != "users" {
		t.Fatalf("expected table users, got %s", c.Table)
	}
	if !c.HasWhereClause {
		t.Fatal("expected HasWhereClause to be true")
	}
	if c.IsDangerous {
		t.Fatal("expected IsDangerous to be false for UPDATE with WHERE")
	}
}

func TestClassifyDelete(t *testing.T) {
	c := ClassifySQL("DELETE FROM logs WHERE created_at < '2024-01-01'")
	if c.Operation != "delete" {
		t.Fatalf("expected delete, got %s", c.Operation)
	}
	if c.Table != "logs" {
		t.Fatalf("expected table logs, got %s", c.Table)
	}
	if !c.HasWhereClause {
		t.Fatal("expected HasWhereClause to be true")
	}
	if c.IsDangerous {
		t.Fatal("expected IsDangerous to be false for DELETE with WHERE")
	}
}

func TestClassifyDropTable(t *testing.T) {
	c := ClassifySQL("DROP TABLE users")
	if c.Operation != "drop_table" {
		t.Fatalf("expected drop_table, got %s", c.Operation)
	}
	if c.Table != "users" {
		t.Fatalf("expected table users, got %s", c.Table)
	}
	if !c.IsDangerous {
		t.Fatal("expected IsDangerous to be true")
	}
}

func TestClassifyDropDatabase(t *testing.T) {
	c := ClassifySQL("DROP DATABASE production")
	if c.Operation != "drop_database" {
		t.Fatalf("expected drop_database, got %s", c.Operation)
	}
	if c.Table != "production" {
		t.Fatalf("expected table production, got %s", c.Table)
	}
	if !c.IsDangerous {
		t.Fatal("expected IsDangerous to be true")
	}
}

func TestClassifyTruncate(t *testing.T) {
	c := ClassifySQL("TRUNCATE TABLE sessions")
	if c.Operation != "truncate" {
		t.Fatalf("expected truncate, got %s", c.Operation)
	}
	if c.Table != "sessions" {
		t.Fatalf("expected table sessions, got %s", c.Table)
	}
	if !c.IsDangerous {
		t.Fatal("expected IsDangerous to be true")
	}
}

func TestClassifyCreateTable(t *testing.T) {
	c := ClassifySQL("CREATE TABLE metrics (id INT PRIMARY KEY, value FLOAT)")
	if c.Operation != "create_table" {
		t.Fatalf("expected create_table, got %s", c.Operation)
	}
	if c.Table != "metrics" {
		t.Fatalf("expected table metrics, got %s", c.Table)
	}
	if c.IsDangerous {
		t.Fatal("expected IsDangerous to be false")
	}
}

func TestClassifyDeleteWithoutWhere(t *testing.T) {
	c := ClassifySQL("DELETE FROM users")
	if c.Operation != "delete" {
		t.Fatalf("expected delete, got %s", c.Operation)
	}
	if !c.IsDangerous {
		t.Fatal("expected IsDangerous to be true for DELETE without WHERE")
	}
	if c.HasWhereClause {
		t.Fatal("expected HasWhereClause to be false")
	}
}

func TestClassifyUpdateWithoutWhere(t *testing.T) {
	c := ClassifySQL("UPDATE users SET active = false")
	if c.Operation != "update" {
		t.Fatalf("expected update, got %s", c.Operation)
	}
	if !c.IsDangerous {
		t.Fatal("expected IsDangerous to be true for UPDATE without WHERE")
	}
	if c.HasWhereClause {
		t.Fatal("expected HasWhereClause to be false")
	}
}

func TestClassifyCaseInsensitive(t *testing.T) {
	cases := []struct {
		query string
		op    string
	}{
		{"select * from users", "select"},
		{"Select Id From Users Where Id = 1", "select"},
		{"INSERT INTO users (name) VALUES ('bob')", "insert"},
		{"drop table Users", "drop_table"},
		{"TRUNCATE TABLE logs", "truncate"},
	}
	for _, tc := range cases {
		c := ClassifySQL(tc.query)
		if c.Operation != tc.op {
			t.Errorf("query %q: expected %s, got %s", tc.query, tc.op, c.Operation)
		}
	}
}
