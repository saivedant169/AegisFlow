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
