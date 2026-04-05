package costopt

import (
	"testing"
)

func TestRecommendationSavingsPercent(t *testing.T) {
	r := Recommendation{
		CurrentModel:          "gpt-4o",
		RecommendedModel:      "gpt-4o-mini",
		CurrentCostPerDay:     500.0,
		RecommendedCostPerDay: 150.0,
		QualityDelta:          -5, // 5% quality drop
	}

	savings := r.SavingsPercent()
	if savings != 70.0 {
		t.Fatalf("expected 70%% savings, got %.1f%%", savings)
	}
}

func TestRecommendationSavingsPerMonth(t *testing.T) {
	r := Recommendation{
		CurrentCostPerDay:     500.0,
		RecommendedCostPerDay: 150.0,
	}
	monthly := r.SavingsPerMonth()
	expected := (500.0 - 150.0) * 30
	if monthly != expected {
		t.Fatalf("expected %.0f, got %.0f", expected, monthly)
	}
}

func TestCostRegistryKnownModel(t *testing.T) {
	reg := DefaultCostRegistry()
	cost, ok := reg.CostPerMillionTokens("gpt-4o")
	if !ok {
		t.Fatal("expected gpt-4o to be registered")
	}
	if cost <= 0 {
		t.Fatalf("expected positive cost, got %f", cost)
	}
}

func TestCostRegistryUnknownModel(t *testing.T) {
	reg := DefaultCostRegistry()
	_, ok := reg.CostPerMillionTokens("unknown-model")
	if ok {
		t.Fatal("expected unknown model to not be registered")
	}
}

func TestCostRegistryCheaperAlternatives(t *testing.T) {
	reg := DefaultCostRegistry()
	alts := reg.CheaperAlternatives("gpt-4o")
	if len(alts) == 0 {
		t.Fatal("expected cheaper alternatives to gpt-4o")
	}
	gpt4oCost, _ := reg.CostPerMillionTokens("gpt-4o")
	for _, alt := range alts {
		altCost, _ := reg.CostPerMillionTokens(alt)
		if altCost >= gpt4oCost {
			t.Fatalf("alternative %s costs %f >= gpt-4o %f", alt, altCost, gpt4oCost)
		}
	}
}

func TestEngineAnalyzeRecommendsDowngrade(t *testing.T) {
	snapshots := []UsageSnapshot{
		{
			TenantID:     "tenant1",
			Model:        "gpt-4o",
			Provider:     "openai",
			RequestCount: 1000,
			TotalTokens:  500000,
			TotalCost:    2.50, // $2.50/day at gpt-4o rates
			AvgQuality:   72,   // decent but not amazing quality
		},
	}

	engine := NewEngine(DefaultCostRegistry(), 10) // min 10% quality delta tolerance
	recs := engine.Analyze(snapshots)

	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation")
	}

	found := false
	for _, r := range recs {
		if r.CurrentModel == "gpt-4o" && r.TenantID == "tenant1" {
			found = true
			if r.SavingsPercent() <= 0 {
				t.Fatal("expected positive savings")
			}
		}
	}
	if !found {
		t.Fatal("expected recommendation for tenant1/gpt-4o")
	}
}

func TestEngineAnalyzeNoRecommendationForCheapModel(t *testing.T) {
	snapshots := []UsageSnapshot{
		{
			TenantID:     "tenant1",
			Model:        "gpt-4o-mini",
			Provider:     "openai",
			RequestCount: 1000,
			TotalTokens:  500000,
			TotalCost:    0.075,
			AvgQuality:   80,
		},
	}

	engine := NewEngine(DefaultCostRegistry(), 10)
	recs := engine.Analyze(snapshots)

	// gpt-4o-mini is already very cheap -- few alternatives
	for _, r := range recs {
		if r.CurrentModel == "gpt-4o-mini" && r.SavingsPercent() < 20 {
			t.Fatal("should not recommend negligible savings")
		}
	}
}

func TestEngineAnalyzeSkipsLowVolume(t *testing.T) {
	snapshots := []UsageSnapshot{
		{
			TenantID:     "tenant1",
			Model:        "gpt-4o",
			Provider:     "openai",
			RequestCount: 5, // too few requests to recommend
			TotalTokens:  2500,
			TotalCost:    0.0125,
			AvgQuality:   80,
		},
	}

	engine := NewEngine(DefaultCostRegistry(), 10)
	recs := engine.Analyze(snapshots)

	if len(recs) > 0 {
		t.Fatal("expected no recommendations for low-volume usage")
	}
}
