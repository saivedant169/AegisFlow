package usage

import (
	"testing"

	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

func TestTrackerUsesProviderModelCosts(t *testing.T) {
	tracker := NewTrackerWithProviders(NewStore(), []config.ProviderConfig{
		{
			Name: "openai",
			ModelCosts: map[string]config.ModelCost{
				"gpt-4o": {InputPer1M: 2.5, OutputPer1M: 10},
			},
		},
	})

	tracker.Record("tenant-1", "openai", "gpt-4o", types.Usage{
		PromptTokens: 1000, CompletionTokens: 2000, TotalTokens: 3000,
	})

	got := tracker.GetUsage("tenant-1")
	cost := got.ByModel["gpt-4o"].EstimatedCostUSD
	want := (1000*2.5 + 2000*10.0) / 1_000_000.0
	if cost != want {
		t.Fatalf("expected cost %f, got %f", want, cost)
	}
}

func TestTrackerUsesProviderDefaultCostFallback(t *testing.T) {
	tracker := NewTrackerWithProviders(NewStore(), []config.ProviderConfig{
		{
			Name:        "openai",
			DefaultCost: config.ModelCost{InputPer1M: 1.5, OutputPer1M: 3.5},
		},
	})

	tracker.Record("tenant-1", "openai", "unknown-model", types.Usage{
		PromptTokens: 1000, CompletionTokens: 2000, TotalTokens: 3000,
	})

	got := tracker.GetUsage("tenant-1")
	cost := got.ByModel["unknown-model"].EstimatedCostUSD
	want := (1000*1.5 + 2000*3.5) / 1_000_000.0
	if cost != want {
		t.Fatalf("expected cost %f, got %f", want, cost)
	}
}

func TestTrackerLegacyFallbackPreserved(t *testing.T) {
	tracker := NewTracker(NewStore())

	tracker.Record("tenant-1", "openai", "gpt-4o", types.Usage{
		PromptTokens: 10, CompletionTokens: 15, TotalTokens: 25,
	})

	got := tracker.GetUsage("tenant-1")
	cost := got.ByModel["gpt-4o"].EstimatedCostUSD
	want := 25 * 5.0 / 1_000_000.0
	if cost != want {
		t.Fatalf("expected cost %f, got %f", want, cost)
	}
}
