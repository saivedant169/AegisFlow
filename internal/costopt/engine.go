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
