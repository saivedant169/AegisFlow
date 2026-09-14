package budget

import "time"

type Forecast struct {
	ScopeID        string  `json:"scope_id"`
	ProjectedTotal float64 `json:"projected_total"`
	DaysRemaining  int     `json:"days_remaining"`
	WillExceed     bool    `json:"will_exceed"`
}

func (m *Manager) Forecast(scopeID string, limit float64) Forecast {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tracker, ok := m.spends[scopeID]
	if !ok || limit <= 0 {
		return Forecast{ScopeID: scopeID}
	}

	now := time.Now()
	daysElapsed := now.Sub(tracker.currentPeriod).Hours() / 24
	if daysElapsed < 1 {
		daysElapsed = 1
	}

	daysInMonth := float64(daysInCurrentMonth(now))
	dailyRate := tracker.accumulated / daysElapsed
	projected := dailyRate * daysInMonth
	remaining := max(int(daysInMonth-daysElapsed), 0)

	return Forecast{
		ScopeID:        scopeID,
		ProjectedTotal: projected,
		DaysRemaining:  remaining,
		WillExceed:     projected > limit,
	}
}

func daysInCurrentMonth(t time.Time) int {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
}
