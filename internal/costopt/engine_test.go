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
