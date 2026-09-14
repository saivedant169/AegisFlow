package analytics

// AdminAdapter wraps a Collector and AlertManager to satisfy the
// admin.AnalyticsProvider interface without creating an import cycle.
type AdminAdapter struct {
	collector    *Collector
	alertManager *AlertManager
}

func NewAdminAdapter(c *Collector, am *AlertManager) *AdminAdapter {
	return &AdminAdapter{collector: c, alertManager: am}
}

func (a *AdminAdapter) RealtimeSummary() map[string]any {
	raw := a.collector.RealtimeSummary()
	result := make(map[string]any)
	for k, v := range raw {
		result[k] = v
	}
	return result
}

func (a *AdminAdapter) RecentAlerts(limit int) any {
	return a.alertManager.RecentAlerts(limit)
}

func (a *AdminAdapter) AcknowledgeAlert(id string) bool {
	return a.alertManager.Acknowledge(id)
}

func (a *AdminAdapter) Dimensions() []string {
	return a.collector.Dimensions()
}
