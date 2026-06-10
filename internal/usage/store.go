package usage

import (
	"sync"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

type ModelUsage struct {
	Model            string  `json:"model"`
	Requests         int64   `json:"requests"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

type ProviderModelUsage struct {
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	Requests         int64   `json:"requests"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

type TenantUsage struct {
	TenantID         string                         `json:"tenant_id"`
	TotalRequests    int64                          `json:"total_requests"`
	TotalTokens      int64                          `json:"total_tokens"`
	EstimatedCostUSD float64                        `json:"estimated_cost_usd"`
	ByModel          map[string]*ModelUsage         `json:"by_model"`
	ByProviderModel  map[string]*ProviderModelUsage `json:"by_provider_model,omitempty"`
}

type Store struct {
	mu      sync.RWMutex
	tenants map[string]*TenantUsage
}

func NewStore() *Store {
	return &Store{
		tenants: make(map[string]*TenantUsage),
	}
}

func (s *Store) Add(tenantID, providerName, model string, u types.Usage, cost float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tenants[tenantID]
	if !ok {
		t = &TenantUsage{
			TenantID:        tenantID,
			ByModel:         make(map[string]*ModelUsage),
			ByProviderModel: make(map[string]*ProviderModelUsage),
		}
		s.tenants[tenantID] = t
	}

	t.TotalRequests++
	t.TotalTokens += int64(u.TotalTokens)
	t.EstimatedCostUSD += cost

	m, ok := t.ByModel[model]
	if !ok {
		m = &ModelUsage{Model: model}
		t.ByModel[model] = m
	}

	m.Requests++
	m.PromptTokens += int64(u.PromptTokens)
	m.CompletionTokens += int64(u.CompletionTokens)
	m.TotalTokens += int64(u.TotalTokens)
	m.EstimatedCostUSD += cost

	providerKey := providerName + "\x00" + model
	pm, ok := t.ByProviderModel[providerKey]
	if !ok {
		pm = &ProviderModelUsage{Provider: providerName, Model: model}
		t.ByProviderModel[providerKey] = pm
	}

	pm.Requests++
	pm.PromptTokens += int64(u.PromptTokens)
	pm.CompletionTokens += int64(u.CompletionTokens)
	pm.TotalTokens += int64(u.TotalTokens)
	pm.EstimatedCostUSD += cost
}

// clone returns a deep copy of a TenantUsage, including its inner maps, so the
// caller can read or JSON-encode it without racing concurrent Add calls.
func (t *TenantUsage) clone() *TenantUsage {
	if t == nil {
		return nil
	}
	c := &TenantUsage{
		TenantID:         t.TenantID,
		TotalRequests:    t.TotalRequests,
		TotalTokens:      t.TotalTokens,
		EstimatedCostUSD: t.EstimatedCostUSD,
		ByModel:          make(map[string]*ModelUsage, len(t.ByModel)),
		ByProviderModel:  make(map[string]*ProviderModelUsage, len(t.ByProviderModel)),
	}
	for k, v := range t.ByModel {
		m := *v
		c.ByModel[k] = &m
	}
	for k, v := range t.ByProviderModel {
		pm := *v
		c.ByProviderModel[k] = &pm
	}
	return c
}

// Get returns a deep copy of one tenant's usage (nil if unknown). Returning the
// live pointer would race the maps mutated by Add.
func (s *Store) Get(tenantID string) *TenantUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tenants[tenantID].clone()
}

// GetAll returns deep copies of every tenant's usage, safe to read or encode
// while Add keeps running.
func (s *Store) GetAll() map[string]*TenantUsage {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*TenantUsage, len(s.tenants))
	for k, v := range s.tenants {
		result[k] = v.clone()
	}
	return result
}
