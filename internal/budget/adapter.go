package budget

// CheckFunc returns a function compatible with the budget middleware.
// It wraps Manager.Check into a plain function signature to avoid
// circular imports between the budget and middleware packages.
func (m *Manager) CheckFunc() func(string, string) (bool, []string, string) {
	return func(tenantID, model string) (bool, []string, string) {
		result := m.Check(tenantID, model)
		return result.Allowed, result.Warnings, result.BlockMsg
	}
}
