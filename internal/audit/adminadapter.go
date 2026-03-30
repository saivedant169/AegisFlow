package audit

// AdminAdapter wraps an audit Logger so it satisfies the admin.AuditProvider interface.
type AdminAdapter struct {
	logger *Logger
}

func NewAdminAdapter(l *Logger) *AdminAdapter {
	return &AdminAdapter{logger: l}
}

func (a *AdminAdapter) Query(actor, action, tenantID string, limit int) (interface{}, error) {
	return a.logger.Query(QueryFilters{
		Actor: actor, Action: action, TenantID: tenantID, Limit: limit,
	})
}

func (a *AdminAdapter) Verify() (interface{}, error) {
	return a.logger.Verify()
}
