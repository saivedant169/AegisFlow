package audit

import (
	"testing"
	"time"
)

func TestLogAndQuery(t *testing.T) {
	store := NewMemoryStore()
	logger, err := NewLogger(store)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Stop()

	logger.Log("admin-key", "admin", "rollout.create", "rollout:r-1", "{}", "tenant-1", "gpt-4o")
	time.Sleep(100 * time.Millisecond) // wait for async writer

	entries, err := logger.Query(QueryFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Action != "rollout.create" {
		t.Errorf("expected rollout.create, got %s", entries[0].Action)
	}
	if entries[0].EntryHash == "" {
		t.Error("expected non-empty hash")
	}
	if entries[0].PreviousHash != "" {
		t.Error("first entry should have empty previous hash")
	}
}

func TestHashChain(t *testing.T) {
	store := NewMemoryStore()
	logger, err := NewLogger(store)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Stop()

	logger.Log("key-1", "admin", "action.one", "res-1", "{}", "t1", "")
	time.Sleep(50 * time.Millisecond)
	logger.Log("key-2", "operator", "action.two", "res-2", "{}", "t1", "")
	time.Sleep(50 * time.Millisecond)

	entries, _ := logger.Query(QueryFilters{})
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[1].PreviousHash != entries[0].EntryHash {
		t.Error("second entry's previous_hash should equal first entry's hash")
	}
}

func TestVerifyIntact(t *testing.T) {
	store := NewMemoryStore()
	logger, err := NewLogger(store)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Stop()

	logger.Log("k1", "admin", "a1", "r1", "{}", "t1", "")
	logger.Log("k2", "admin", "a2", "r2", "{}", "t1", "")
	time.Sleep(100 * time.Millisecond)

	result, err := logger.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Errorf("expected valid, got: %s", result.Message)
	}
}

func TestVerifyEmpty(t *testing.T) {
	store := NewMemoryStore()
	logger, err := NewLogger(store)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Stop()

	result, _ := logger.Verify()
	if !result.Valid {
		t.Error("empty log should be valid")
	}
}

func TestMultipleSequentialEntriesChain(t *testing.T) {
	store := NewMemoryStore()
	logger, err := NewLogger(store)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Stop()

	for i := 0; i < 5; i++ {
		logger.Log("actor", "admin", "action", "resource", "{}", "t1", "")
		time.Sleep(30 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)

	entries, err := logger.Query(QueryFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	// Verify chain: each entry's PreviousHash == previous entry's EntryHash
	if entries[0].PreviousHash != "" {
		t.Error("first entry should have empty PreviousHash")
	}
	for i := 1; i < len(entries); i++ {
		if entries[i].PreviousHash != entries[i-1].EntryHash {
			t.Errorf("entry %d: PreviousHash mismatch (expected %s, got %s)",
				i, entries[i-1].EntryHash, entries[i].PreviousHash)
		}
	}
}

func TestVerifyDetectsTamperedDetail(t *testing.T) {
	store := NewMemoryStore()
	logger, err := NewLogger(store)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Stop()

	logger.Log("k1", "admin", "a1", "r1", "original", "t1", "")
	logger.Log("k2", "admin", "a2", "r2", "original", "t1", "")
	time.Sleep(100 * time.Millisecond)

	// Tamper with the first entry's detail directly in the store.
	store.mu.Lock()
	store.entries[0].Detail = "tampered"
	store.mu.Unlock()

	result, err := logger.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatal("expected verify to detect tampered entry")
	}
	if result.ErrorAt != 1 {
		t.Errorf("expected error at entry 1, got %d", result.ErrorAt)
	}
}

func TestVerifyDetectsBrokenChain(t *testing.T) {
	store := NewMemoryStore()
	logger, err := NewLogger(store)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Stop()

	logger.Log("k1", "admin", "a1", "r1", "{}", "t1", "")
	logger.Log("k2", "admin", "a2", "r2", "{}", "t1", "")
	logger.Log("k3", "admin", "a3", "r3", "{}", "t1", "")
	time.Sleep(150 * time.Millisecond)

	// Break the chain: modify second entry's PreviousHash
	store.mu.Lock()
	store.entries[1].PreviousHash = "bogus_hash"
	// Also recompute the entry hash so the hash-mismatch check passes,
	// but the chain-break check should still catch it.
	store.entries[1].EntryHash = computeHash(store.entries[1])
	store.mu.Unlock()

	result, err := logger.Verify()
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatal("expected verify to detect broken chain")
	}
}

func TestLogQueueFullBehavior(t *testing.T) {
	store := NewMemoryStore()
	logger, err := NewLogger(store)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Stop()

	// The queue has capacity 1024. Send 1100 entries rapidly.
	// Some should be dropped (no panic, no blocking).
	for i := 0; i < 1100; i++ {
		logger.Log("actor", "admin", "action", "resource", "{}", "t1", "")
	}

	// Give the writer time to drain.
	time.Sleep(500 * time.Millisecond)

	entries, err := logger.Query(QueryFilters{})
	if err != nil {
		t.Fatal(err)
	}
	// We should have at most 1024 entries (queue capacity) and at least some entries.
	if len(entries) == 0 {
		t.Fatal("expected some entries to be written")
	}
	if len(entries) > 1100 {
		t.Fatalf("entries exceed what was sent: %d", len(entries))
	}
	t.Logf("queue full test: %d of 1100 entries written", len(entries))
}

func TestMemoryStoreQueryReturnsAllInOrder(t *testing.T) {
	store := NewMemoryStore()
	// Directly insert entries to test the store independently.
	for i := 0; i < 5; i++ {
		_ = store.Insert(Entry{
			Actor:  "actor",
			Action: "action",
			Detail: "{}",
		})
	}

	entries, err := store.Query(QueryFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
	// Verify IDs are sequential (1-based).
	for i, e := range entries {
		if e.ID != int64(i+1) {
			t.Errorf("expected ID %d, got %d", i+1, e.ID)
		}
	}
}

func TestComputeHashDeterministic(t *testing.T) {
	e := Entry{
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Actor:     "test", ActorRole: "admin", Action: "test.action",
		Resource: "res-1", Detail: "{}", TenantID: "t1", PreviousHash: "",
	}
	h1 := computeHash(e)
	h2 := computeHash(e)
	if h1 != h2 {
		t.Error("hash should be deterministic")
	}
	if len(h1) != 64 {
		t.Errorf("expected 64 char hex hash, got %d", len(h1))
	}
}

func TestComputeHashDifferentInputsDifferentHash(t *testing.T) {
	e1 := Entry{
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Actor:     "test", ActorRole: "admin", Action: "action.a",
		Resource: "res-1", Detail: "{}", TenantID: "t1", PreviousHash: "",
	}
	e2 := Entry{
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Actor:     "test", ActorRole: "admin", Action: "action.b",
		Resource: "res-1", Detail: "{}", TenantID: "t1", PreviousHash: "",
	}
	if computeHash(e1) == computeHash(e2) {
		t.Error("different entries should produce different hashes")
	}
}

func TestMemoryStoreLastHashWithEntries(t *testing.T) {
	store := NewMemoryStore()
	logger, err := NewLogger(store)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Stop()

	logger.Log("k1", "admin", "a1", "r1", "{}", "t1", "")
	time.Sleep(100 * time.Millisecond)

	hash, err := store.LastHash()
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Error("expected non-empty last hash after inserting an entry")
	}
	if len(hash) != 64 {
		t.Errorf("expected 64 char hex hash, got %d chars", len(hash))
	}
}
