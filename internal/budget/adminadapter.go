package budget

// AdminAdapter wraps a *Manager so it satisfies the admin.BudgetProvider
// interface without creating an import cycle.
type AdminAdapter struct {
	manager *Manager
	scopes  []SpendScope
}

// NewAdminAdapter creates an AdminAdapter that bridges the budget Manager to the
// admin server's BudgetProvider interface.
func NewAdminAdapter(m *Manager, scopes []SpendScope) *AdminAdapter {
	return &AdminAdapter{manager: m, scopes: scopes}
}

// AllStatuses returns the current spend status for every configured scope.
func (a *AdminAdapter) AllStatuses() interface{} {
	return a.manager.AllStatuses()
}

// ForecastAll returns a forecast for every configured scope.
func (a *AdminAdapter) ForecastAll() interface{} {
	var forecasts []Forecast
	for _, s := range a.scopes {
		forecasts = append(forecasts, a.manager.Forecast(s.ScopeID, s.Limit))
	}
	return forecasts
}
