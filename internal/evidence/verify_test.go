package evidence

import (
	"testing"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func TestVerifyValidChain(t *testing.T) {
	chain := NewSessionChain("s1")
	chain.Record(testEnv("t1", envelope.DecisionAllow))
	chain.Record(testEnv("t2", envelope.DecisionReview))
	chain.Record(testEnv("t3", envelope.DecisionBlock))

	result := Verify(chain.Records())
	if !result.Valid {
		t.Fatalf("expected valid chain, got: %s", result.Message)
	}
	if result.TotalRecords != 3 {
		t.Fatalf("expected 3 records, got %d", result.TotalRecords)
	}
}

func TestVerifyEmptyChain(t *testing.T) {
	result := Verify(nil)
	if !result.Valid {
		t.Fatal("empty chain should be valid")
	}
}

func TestVerifyDetectsTamperedHash(t *testing.T) {
	chain := NewSessionChain("s1")
	chain.Record(testEnv("t1", envelope.DecisionAllow))
	chain.Record(testEnv("t2", envelope.DecisionAllow))

	records := chain.Records()
	records[0].Hash = "tampered"

	result := Verify(records)
	if result.Valid {
		t.Fatal("expected invalid for tampered hash")
	}
	if result.ErrorAtIndex != 0 {
		t.Fatalf("expected error at index 0, got %d", result.ErrorAtIndex)
	}
}

func TestVerifyDetectsBrokenLink(t *testing.T) {
	chain := NewSessionChain("s1")
	chain.Record(testEnv("t1", envelope.DecisionAllow))
	chain.Record(testEnv("t2", envelope.DecisionAllow))

	records := chain.Records()
	records[1].PreviousHash = "wrong"

	result := Verify(records)
	if result.Valid {
		t.Fatal("expected invalid for broken chain link")
	}
	if result.ErrorAtIndex != 1 {
		t.Fatalf("expected error at index 1, got %d", result.ErrorAtIndex)
	}
}
