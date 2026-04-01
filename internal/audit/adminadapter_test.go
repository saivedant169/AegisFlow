package audit

import (
	"testing"
	"time"
)

func TestAdminAdapterQuery(t *testing.T) {
	store := NewMemoryStore()
	logger, err := NewLogger(store)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Stop()

	logger.Log("admin", "admin", "test.action", "resource", "{}", "t1", "")
	time.Sleep(100 * time.Millisecond)

	adapter := NewAdminAdapter(logger)
	result, err := adapter.Query("", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	entries, ok := result.([]Entry)
	if !ok {
		t.Fatal("expected []Entry")
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestAdminAdapterVerify(t *testing.T) {
	store := NewMemoryStore()
	logger, err := NewLogger(store)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Stop()

	logger.Log("admin", "admin", "test.action", "resource", "{}", "t1", "")
	time.Sleep(100 * time.Millisecond)

	adapter := NewAdminAdapter(logger)
	result, err := adapter.Verify()
	if err != nil {
		t.Fatal(err)
	}
	vr, ok := result.(VerifyResult)
	if !ok {
		t.Fatal("expected VerifyResult")
	}
	if !vr.Valid {
		t.Errorf("expected valid audit log, got: %s", vr.Message)
	}
}
