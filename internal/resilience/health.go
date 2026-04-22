package resilience

import (
	"sync"
	"time"
)

// ComponentHealth represents the health status of a single system component.
type ComponentHealth struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"` // healthy, degraded, unhealthy
	LastCheck time.Time `json:"last_check"`
	Message   string    `json:"message,omitempty"`
	SafeMode  string    `json:"safe_mode"` // what happens when this component fails
}

// HealthChecker is implemented by components that can report their health.
type HealthChecker interface {
	Check() ComponentHealth
}

// HealthRegistry tracks health checkers for all registered components.
type HealthRegistry struct {
	mu       sync.RWMutex
	checkers map[string]HealthChecker
}

// NewHealthRegistry creates a new HealthRegistry.
func NewHealthRegistry() *HealthRegistry {
	return &HealthRegistry{
		checkers: make(map[string]HealthChecker),
	}
}

// Register adds a named health checker.
func (r *HealthRegistry) Register(name string, checker HealthChecker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkers[name] = checker
}

// CheckAll runs all registered health checks and returns their results.
func (r *HealthRegistry) CheckAll() []ComponentHealth {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]ComponentHealth, 0, len(r.checkers))
	for _, checker := range r.checkers {
		results = append(results, checker.Check())
	}
	return results
}

// IsHealthy returns true only if every component reports healthy.
func (r *HealthRegistry) IsHealthy() bool {
	for _, h := range r.CheckAll() {
		if h.Status != "healthy" {
			return false
		}
	}
	return true
}

// IsDegraded returns true if at least one component is degraded or unhealthy
// but not all are unhealthy.
func (r *HealthRegistry) IsDegraded() bool {
	results := r.CheckAll()
	if len(results) == 0 {
		return false
	}
	hasDegraded := false
	allUnhealthy := true
	for _, h := range results {
		if h.Status == "degraded" || h.Status == "unhealthy" {
			hasDegraded = true
		}
		if h.Status != "unhealthy" {
			allUnhealthy = false
		}
	}
	if allUnhealthy {
		return false // fully down, not merely degraded
	}
	return hasDegraded
}
