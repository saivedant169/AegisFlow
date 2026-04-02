package router

import (
	"sync/atomic"

	"github.com/saivedant169/AegisFlow/internal/provider"
)

type Strategy interface {
	Select(providers []provider.Provider) []provider.Provider
}

type PriorityStrategy struct{}

func (s *PriorityStrategy) Select(providers []provider.Provider) []provider.Provider {
	return providers
}

type RoundRobinStrategy struct {
	counter uint64
}

func (s *RoundRobinStrategy) Select(providers []provider.Provider) []provider.Provider {
	if len(providers) == 0 {
		return providers
	}
	idx := atomic.AddUint64(&s.counter, 1) % uint64(len(providers))
	result := make([]provider.Provider, len(providers))
	for i := range providers {
		result[i] = providers[(int(idx)+i)%len(providers)]
	}
	return result
}

func NewStrategy(name string) Strategy {
	switch name {
	case "round-robin":
		return &RoundRobinStrategy{}
	default:
		return &PriorityStrategy{}
	}
}
