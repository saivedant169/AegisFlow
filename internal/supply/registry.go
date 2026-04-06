package supply

import (
	"sync"
	"time"
)

// LoadedAsset tracks an extension that has been loaded into the runtime.
type LoadedAsset struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Type        string    `json:"type"`
	TrustTier   string    `json:"trust_tier"`
	LoadedAt    time.Time `json:"loaded_at"`
	ContentHash string    `json:"content_hash"`
	Verified    bool      `json:"verified"`
}

// Registry is an in-memory store of loaded assets with their trust status.
type Registry struct {
	mu     sync.RWMutex
	assets []LoadedAsset
}

// NewRegistry creates an empty asset registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds an asset to the registry, stamping LoadedAt to now.
func (r *Registry) Register(asset LoadedAsset) {
	r.mu.Lock()
	defer r.mu.Unlock()
	asset.LoadedAt = time.Now().UTC()
	r.assets = append(r.assets, asset)
}

// ListAssets returns a snapshot of all loaded assets.
func (r *Registry) ListAssets() []LoadedAsset {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]LoadedAsset, len(r.assets))
	copy(out, r.assets)
	return out
}

// CountByTrustTier returns counts of assets grouped by trust tier.
func (r *Registry) CountByTrustTier() map[string]int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	counts := make(map[string]int)
	for _, a := range r.assets {
		counts[a.TrustTier]++
	}
	return counts
}
