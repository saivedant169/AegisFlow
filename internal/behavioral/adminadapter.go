package behavioral

// AdminAdapter wraps a behavioral Registry to satisfy the admin.BehavioralProvider interface.
type AdminAdapter struct {
	registry *Registry
}

// NewAdminAdapter creates a new AdminAdapter wrapping the given Registry.
func NewAdminAdapter(r *Registry) *AdminAdapter {
	return &AdminAdapter{registry: r}
}

// SessionRisk returns the risk score and alerts for a session.
func (a *AdminAdapter) SessionRisk(sessionID string) (interface{}, error) {
	sa := a.registry.Get(sessionID)
	if sa == nil {
		return nil, nil
	}
	return map[string]interface{}{
		"session_id": sessionID,
		"risk_score": sa.SessionRiskScore(),
		"blocked":    sa.Blocked(),
		"alerts":     sa.Alerts(),
	}, nil
}

// ListSessions returns all tracked session IDs.
func (a *AdminAdapter) ListSessions() interface{} {
	return a.registry.ListSessions()
}
