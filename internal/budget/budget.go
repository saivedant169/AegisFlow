package budget

import (
	"fmt"
	"sync"
	"time"
)

type SpendScope struct {
	Scope   string // "global", "tenant", "tenant_model"
	ScopeID string // "global", "premium", "premium:gpt-4o"
	Limit   float64
	AlertAt int // percentage
	WarnAt  int // percentage
}

type SpendStatus struct {
	Scope        string  `json:"scope"`
	ScopeID      string  `json:"scope_id"`
	Limit        float64 `json:"limit"`
	CurrentSpend float64 `json:"current_spend"`
	Percentage   float64 `json:"percentage"`
	Status       string  `json:"status"` // "normal", "alert", "warn", "blocked"
	AlertAt      int     `json:"alert_at"`
	WarnAt       int     `json:"warn_at"`
}

type CheckResult struct {
	Allowed  bool
	Warnings []string
	BlockMsg string
}

type Manager struct {
	mu     sync.RWMutex
	spends map[string]*spendTracker // keyed by scopeID
	scopes []SpendScope
}

type spendTracker struct {
	currentPeriod time.Time // start of current budget period
	accumulated   float64
}

func NewManager(scopes []SpendScope) *Manager {
	m := &Manager{
		spends: make(map[string]*spendTracker),
		scopes: scopes,
	}
	now := time.Now()
	for _, s := range scopes {
		m.spends[s.ScopeID] = &spendTracker{
			currentPeriod: startOfMonth(now),
		}
	}
	return m
}

func (m *Manager) RecordSpend(tenantID, model string, cost float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()

	keys := []string{"global", tenantID, tenantID + ":" + model}
	for _, key := range keys {
		tracker, ok := m.spends[key]
		if !ok {
			tracker = &spendTracker{currentPeriod: startOfMonth(now)}
			m.spends[key] = tracker
		}
		// Reset if new period
		if now.Month() != tracker.currentPeriod.Month() || now.Year() != tracker.currentPeriod.Year() {
			tracker.accumulated = 0
			tracker.currentPeriod = startOfMonth(now)
		}
		tracker.accumulated += cost
	}
}

func (m *Manager) Check(tenantID, model string) CheckResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := CheckResult{Allowed: true}

	for _, scope := range m.scopes {
		var key string
		switch scope.Scope {
		case "global":
			key = "global"
		case "tenant":
			if scope.ScopeID != tenantID {
				continue
			}
			key = tenantID
		case "tenant_model":
			expected := tenantID + ":" + model
			if scope.ScopeID != expected {
				continue
			}
			key = expected
		}

		tracker, ok := m.spends[key]
		if !ok {
			continue
		}

		if scope.Limit <= 0 {
			continue
		}

		pct := tracker.accumulated / scope.Limit * 100

		if pct >= 100 {
			result.Allowed = false
			result.BlockMsg = fmt.Sprintf("%s budget of $%.2f exhausted (current: $%.2f)", scope.Scope, scope.Limit, tracker.accumulated)
			return result
		}
		if pct >= float64(scope.WarnAt) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s budget %.0f%% used", scope.Scope, pct))
		}
	}

	return result
}

func (m *Manager) AllStatuses() []SpendStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var statuses []SpendStatus
	for _, scope := range m.scopes {
		tracker, ok := m.spends[scope.ScopeID]
		if !ok {
			continue
		}
		pct := 0.0
		if scope.Limit > 0 {
			pct = tracker.accumulated / scope.Limit * 100
		}
		status := "normal"
		if pct >= 100 {
			status = "blocked"
		} else if pct >= float64(scope.WarnAt) {
			status = "warn"
		} else if pct >= float64(scope.AlertAt) {
			status = "alert"
		}
		statuses = append(statuses, SpendStatus{
			Scope: scope.Scope, ScopeID: scope.ScopeID,
			Limit: scope.Limit, CurrentSpend: tracker.accumulated,
			Percentage: pct, Status: status,
			AlertAt: scope.AlertAt, WarnAt: scope.WarnAt,
		})
	}
	return statuses
}

func startOfMonth(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}
