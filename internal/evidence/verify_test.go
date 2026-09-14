package evidence

import (
	"testing"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

func TestVerifyValidChain(t *testing.T) {
	chain := NewSessionChain("s1")
	_, err := chain.Record(testEnv("t1", envelope.DecisionAllow))
	if err != nil {
		return
	}
	_, err = chain.Record(testEnv("t2", envelope.DecisionReview))
	if err != nil {
		return
	}
	_, err = chain.Record(testEnv("t3", envelope.DecisionBlock))
	if err != nil {
		return
	}

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

func TestVerifyRejectsMissingFirstRecord(t *testing.T) {
	chain := NewSessionChain("missing-first")
	if _, err := chain.Record(testEnv("record-1", envelope.DecisionReview)); err != nil {
		t.Fatal(err)
	}
	if _, err := chain.Record(testEnv("record-2", envelope.DecisionReview)); err != nil {
		t.Fatal(err)
	}
	records := chain.Records()[1:]
	if result := Verify(records); result.Valid {
		t.Fatal("chain without its first record passed verification")
	}
}

func TestVerifyRejectsChangedRecordIndex(t *testing.T) {
	chain := NewSessionChain("changed-index")
	if _, err := chain.Record(testEnv("record-1", envelope.DecisionReview)); err != nil {
		t.Fatal(err)
	}
	records := chain.Records()
	records[0].Index = 7
	if result := Verify(records); result.Valid {
		t.Fatal("chain with changed record index passed verification")
	}
}

func TestVerifyDetectsTamperedHash(t *testing.T) {
	chain := NewSessionChain("s1")
	_, err := chain.Record(testEnv("t1", envelope.DecisionAllow))
	if err != nil {
		return
	}
	_, err = chain.Record(testEnv("t2", envelope.DecisionAllow))
	if err != nil {
		return
	}

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
	_, err := chain.Record(testEnv("t1", envelope.DecisionAllow))
	if err != nil {
		return
	}
	_, err = chain.Record(testEnv("t2", envelope.DecisionAllow))
	if err != nil {
		return
	}

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
