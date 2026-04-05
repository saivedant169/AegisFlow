package costopt

// Recommendation suggests a cheaper model for a tenant's workload.
type Recommendation struct {
	TenantID              string  `json:"tenant_id"`
	CurrentModel          string  `json:"current_model"`
	CurrentProvider       string  `json:"current_provider"`
	RecommendedModel      string  `json:"recommended_model"`
	RecommendedProvider   string  `json:"recommended_provider"`
	CurrentCostPerDay     float64 `json:"current_cost_per_day"`
	RecommendedCostPerDay float64 `json:"recommended_cost_per_day"`
	RequestsPerDay        int     `json:"requests_per_day"`
	QualityDelta          int     `json:"quality_delta"` // negative means quality drops
	Reason                string  `json:"reason"`
}

func (r *Recommendation) SavingsPercent() float64 {
	if r.CurrentCostPerDay == 0 {
		return 0
	}
	return (1 - r.RecommendedCostPerDay/r.CurrentCostPerDay) * 100
}

func (r *Recommendation) SavingsPerMonth() float64 {
	return (r.CurrentCostPerDay - r.RecommendedCostPerDay) * 30
}

// CostRegistry maps model names to their cost per million tokens.
type CostRegistry struct {
	models map[string]float64
}

func DefaultCostRegistry() *CostRegistry {
	return &CostRegistry{
		models: map[string]float64{
			"gpt-4o":          5.00,
			"gpt-4o-mini":     0.15,
			"gpt-4-turbo":     10.00,
			"gpt-3.5-turbo":   0.50,
			"claude-sonnet-4": 3.00,
			"claude-haiku":    0.25,
			"claude-opus-4":   15.00,
			"gemini-pro":      0.50,
			"gemini-flash":    0.075,
			"llama3-70b":      0.59,
			"llama3-8b":       0.05,
			"mistral-large":   2.00,
			"mistral-small":   0.20,
			"mixtral-8x7b":    0.24,
		},
	}
}

func (cr *CostRegistry) CostPerMillionTokens(model string) (float64, bool) {
	cost, ok := cr.models[model]
	return cost, ok
}

// CheaperAlternatives returns models that cost less than the given model,
// sorted cheapest first.
func (cr *CostRegistry) CheaperAlternatives(model string) []string {
	cost, ok := cr.models[model]
	if !ok {
		return nil
	}
	var alts []string
	for name, c := range cr.models {
		if name != model && c < cost {
			alts = append(alts, name)
		}
	}
	// Sort by cost ascending
	for i := 0; i < len(alts); i++ {
		for j := i + 1; j < len(alts); j++ {
			if cr.models[alts[j]] < cr.models[alts[i]] {
				alts[i], alts[j] = alts[j], alts[i]
			}
		}
	}
	return alts
}
