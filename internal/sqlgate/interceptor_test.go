package sqlgate

import (
	"testing"

	"github.com/saivedant169/AegisFlow/internal/evidence"
	"github.com/saivedant169/AegisFlow/internal/toolpolicy"
)

// newTestInterceptor builds an Interceptor with the standard SQL rules from
// the example config: read=allow, write=review, delete=block.
func newTestInterceptor(blockDangerous bool) *Interceptor {
	rules := []toolpolicy.ToolRule{
		{Protocol: "sql", Tool: "*", Capability: "read", Decision: "allow"},
		{Protocol: "sql", Tool: "*", Capability: "write", Decision: "review"},
		{Protocol: "sql", Tool: "*", Capability: "delete", Decision: "block"},
	}
	engine := toolpolicy.NewEngine(rules, "block")
	chain := evidence.NewSessionChain("test-session")
	return NewInterceptor(engine, chain, blockDangerous)
}

func TestAllowSelect(t *testing.T) {
	interceptor := newTestInterceptor(false)
	res, err := interceptor.Evaluate("SELECT * FROM users WHERE id = 1", "mydb")
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != "allow" {
		t.Fatalf("expected allow, got %s", res.Decision)
	}
	if res.Operation != "select" {
		t.Fatalf("expected select, got %s", res.Operation)
	}
}

func TestBlockDropTable(t *testing.T) {
	interceptor := newTestInterceptor(false)
	res, err := interceptor.Evaluate("DROP TABLE users", "mydb")
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != "block" {
		t.Fatalf("expected block, got %s", res.Decision)
	}
	if res.Operation != "drop_table" {
		t.Fatalf("expected drop_table, got %s", res.Operation)
	}
}

func TestBlockDropDatabase(t *testing.T) {
	interceptor := newTestInterceptor(false)
	res, err := interceptor.Evaluate("DROP DATABASE production", "production")
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != "block" {
		t.Fatalf("expected block, got %s", res.Decision)
	}
	if res.Operation != "drop_database" {
		t.Fatalf("expected drop_database, got %s", res.Operation)
	}
}

func TestReviewInsert(t *testing.T) {
	interceptor := newTestInterceptor(false)
	res, err := interceptor.Evaluate("INSERT INTO orders (product) VALUES ('widget')", "mydb")
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != "review" {
		t.Fatalf("expected review, got %s", res.Decision)
	}
	if res.Operation != "insert" {
		t.Fatalf("expected insert, got %s", res.Operation)
	}
}

func TestReviewDeleteWithWhere(t *testing.T) {
	// With blockDangerous=false, a DELETE with WHERE should match the delete
	// capability rule and get "block" from the policy engine (not dangerous-block).
	interceptor := newTestInterceptor(false)
	res, err := interceptor.Evaluate("DELETE FROM logs WHERE created_at < '2024-01-01'", "mydb")
	if err != nil {
		t.Fatal(err)
	}
	// delete capability maps to "block" in our test rules
	if res.Decision != "block" {
		t.Fatalf("expected block, got %s", res.Decision)
	}
}

func TestBlockDeleteWithoutWhere(t *testing.T) {
	// With blockDangerous=true, DELETE without WHERE is auto-blocked before
	// policy evaluation.
	interceptor := newTestInterceptor(true)
	res, err := interceptor.Evaluate("DELETE FROM users", "mydb")
	if err != nil {
		t.Fatal(err)
	}
	if res.Decision != "block" {
		t.Fatalf("expected block, got %s", res.Decision)
	}
	if res.Message != "dangerous SQL operation: delete" {
		t.Fatalf("unexpected message: %s", res.Message)
	}
}

func TestEvidenceRecorded(t *testing.T) {
	rules := []toolpolicy.ToolRule{
		{Protocol: "sql", Tool: "*", Capability: "read", Decision: "allow"},
	}
	engine := toolpolicy.NewEngine(rules, "block")
	chain := evidence.NewSessionChain("evidence-test")
	interceptor := NewInterceptor(engine, chain, false)

	_, err := interceptor.Evaluate("SELECT 1", "testdb")
	if err != nil {
		t.Fatal(err)
	}
	_, err = interceptor.Evaluate("SELECT 2", "testdb")
	if err != nil {
		t.Fatal(err)
	}

	if chain.Count() != 2 {
		t.Fatalf("expected 2 evidence records, got %d", chain.Count())
	}
}

func TestDatabaseInTarget(t *testing.T) {
	interceptor := newTestInterceptor(false)
	res, err := interceptor.Evaluate("SELECT 1", "analytics_db")
	if err != nil {
		t.Fatal(err)
	}
	if res.EnvelopeID == "" {
		t.Fatal("expected non-empty EnvelopeID")
	}
	// Verify the evidence chain recorded the correct target.
	records := interceptor.chain.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Envelope.Target != "analytics_db" {
		t.Fatalf("expected target analytics_db, got %s", records[0].Envelope.Target)
	}
}
