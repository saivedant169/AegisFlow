package usage

import (
	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

var legacyCostPerMillionTokens = map[string]float64{
	"gpt-4o":                   5.0,
	"gpt-4o-mini":              0.15,
	"claude-sonnet-4-20250514": 3.0,
	"llama3":                   0.0,
	"mock":                     0.0,
	"mock-fast":                0.0,
}

type Tracker struct {
	store          *Store
	providerConfig map[string]config.ProviderConfig
}

func NewTracker(store *Store) *Tracker {
	return &Tracker{store: store}
}

func NewTrackerWithProviders(store *Store, providers []config.ProviderConfig) *Tracker {
	providerConfig := make(map[string]config.ProviderConfig, len(providers))
	for _, provider := range providers {
		providerConfig[provider.Name] = provider
	}
	return &Tracker{store: store, providerConfig: providerConfig}
}

func (t *Tracker) Record(tenantID, providerName, model string, usage types.Usage) {
	cost := t.estimateCost(providerName, model, usage)
	t.store.Add(tenantID, model, usage, cost)
}

func (t *Tracker) GetUsage(tenantID string) *TenantUsage {
	return t.store.Get(tenantID)
}

func (t *Tracker) GetAllUsage() map[string]*TenantUsage {
	return t.store.GetAll()
}

func (t *Tracker) estimateCost(providerName, model string, usage types.Usage) float64 {
	if provider, ok := t.providerConfig[providerName]; ok {
		if cost, ok := provider.ModelCosts[model]; ok {
			return (float64(usage.PromptTokens)*cost.InputPer1M + float64(usage.CompletionTokens)*cost.OutputPer1M) / 1_000_000.0
		}
		if provider.DefaultCost.InputPer1M > 0 || provider.DefaultCost.OutputPer1M > 0 {
			return (float64(usage.PromptTokens)*provider.DefaultCost.InputPer1M + float64(usage.CompletionTokens)*provider.DefaultCost.OutputPer1M) / 1_000_000.0
		}
	}

	rate, ok := legacyCostPerMillionTokens[model]
	if !ok {
		rate = 1.0
	}
	return float64(usage.TotalTokens) / 1_000_000.0 * rate
}

func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return len(text) / 4
}
