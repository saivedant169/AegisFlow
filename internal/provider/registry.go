package provider

import (
	"fmt"
	"sync"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.Name()] = p
}

func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("provider %q not found", name)
	}
	return p, nil
}

func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		result = append(result, p)
	}
	return result
}

func (r *Registry) AllModels() []types.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var models []types.Model
	for _, p := range r.providers {
		m, err := p.Models(nil)
		if err == nil {
			models = append(models, m...)
		}
	}
	return models
}
