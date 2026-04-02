package usage

import (
	"github.com/saivedant169/AegisFlow/pkg/types"
)

var costPerMillionTokens = map[string]float64{
	"gpt-4o":                    5.0,
	"gpt-4o-mini":               0.15,
	"claude-sonnet-4-20250514":  3.0,
	"llama3":                    0.0,
	"mock":                      0.0,
	"mock-fast":                 0.0,
}

type Tracker struct {
	store *Store
}

func NewTracker(store *Store) *Tracker {
	return &Tracker{store: store}
}

func (t *Tracker) Record(tenantID, model string, usage types.Usage) {
	cost := estimateCost(model, usage.TotalTokens)
	t.store.Add(tenantID, model, usage, cost)
}

func (t *Tracker) GetUsage(tenantID string) *TenantUsage {
	return t.store.Get(tenantID)
}

func (t *Tracker) GetAllUsage() map[string]*TenantUsage {
	return t.store.GetAll()
}

func estimateCost(model string, tokens int) float64 {
	rate, ok := costPerMillionTokens[model]
	if !ok {
		rate = 1.0
	}
	return float64(tokens) / 1_000_000.0 * rate
}

func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}
