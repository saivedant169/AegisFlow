package usage

import (
	"testing"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestTrackerRecordAndGet(t *testing.T) {
	tracker := NewTracker(NewStore())
	u := types.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}

	tracker.Record("tenant-1", "openai", "gpt-4o", u)

	got := tracker.GetUsage("tenant-1")
	if got == nil {
		t.Fatal("expected usage for tenant-1")
	}
	if got.TotalTokens != 150 {
		t.Fatalf("expected 150 tokens, got %d", got.TotalTokens)
	}
}

func TestTrackerGetUsageNotFound(t *testing.T) {
	tracker := NewTracker(NewStore())
	got := tracker.GetUsage("nonexistent")
	if got != nil {
		t.Fatal("expected nil for unknown tenant")
	}
}

func TestTrackerGetAllUsage(t *testing.T) {
	tracker := NewTracker(NewStore())
	u := types.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}

	tracker.Record("t1", "openai", "gpt-4o", u)
	tracker.Record("t2", "anthropic", "claude-sonnet-4-20250514", u)

	all := tracker.GetAllUsage()
	if len(all) != 2 {
		t.Fatalf("expected 2 tenants, got %d", len(all))
	}
	if all["t1"] == nil || all["t2"] == nil {
		t.Fatal("expected both tenants in usage map")
	}
}

func TestEstimateCostKnownModel(t *testing.T) {
	// gpt-4o costs 5.0 per million tokens
	cost := estimateCost("gpt-4o", 1_000_000)
	if cost != 5.0 {
		t.Fatalf("expected 5.0, got %f", cost)
	}
}

func TestEstimateCostUnknownModel(t *testing.T) {
	// Unknown model defaults to 1.0 per million
	cost := estimateCost("unknown-model", 1_000_000)
	if cost != 1.0 {
		t.Fatalf("expected 1.0, got %f", cost)
	}
}

func TestEstimateCostFreeModel(t *testing.T) {
	cost := estimateCost("mock", 1_000_000)
	if cost != 0.0 {
		t.Fatalf("expected 0.0, got %f", cost)
	}
}

func TestEstimateTokens(t *testing.T) {
	// ~4 chars per token
	tokens := EstimateTokens("hello world!") // 12 chars = 3 tokens
	if tokens != 3 {
		t.Fatalf("expected 3, got %d", tokens)
	}
}

func TestEstimateTokensEmpty(t *testing.T) {
	tokens := EstimateTokens("")
	if tokens != 0 {
		t.Fatalf("expected 0, got %d", tokens)
	}
}

func TestTrackerCostAccumulation(t *testing.T) {
	tracker := NewTracker(NewStore())
	u := types.Usage{PromptTokens: 500, CompletionTokens: 500, TotalTokens: 1000}

	tracker.Record("t1", "openai", "gpt-4o", u)
	tracker.Record("t1", "openai", "gpt-4o", u)

	got := tracker.GetUsage("t1")
	if got.TotalTokens != 2000 {
		t.Fatalf("expected 2000 tokens, got %d", got.TotalTokens)
	}
	// 2000 tokens at 5.0/M = 0.01
	expectedCost := 0.01
	if got.EstimatedCostUSD < expectedCost-0.001 || got.EstimatedCostUSD > expectedCost+0.001 {
		t.Fatalf("expected cost ~%f, got %f", expectedCost, got.EstimatedCostUSD)
	}
}
