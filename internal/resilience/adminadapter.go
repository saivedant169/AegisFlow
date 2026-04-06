package resilience

// AdminAdapter exposes resilience subsystems to the admin API without
// creating import cycles.
type AdminAdapter struct {
	health      *HealthRegistry
	degradation *DegradationManager
	retention   *RetentionManager
	backup      *BackupManager
}

// NewAdminAdapter creates an AdminAdapter wrapping all resilience managers.
func NewAdminAdapter(h *HealthRegistry, d *DegradationManager, r *RetentionManager, b *BackupManager) *AdminAdapter {
	return &AdminAdapter{health: h, degradation: d, retention: r, backup: b}
}

// DetailedHealth returns the health status of all registered components.
func (a *AdminAdapter) DetailedHealth() interface{} {
	return a.health.CheckAll()
}

// DegradationModes returns current degradation state for every component.
func (a *AdminAdapter) DegradationModes() interface{} {
	return a.degradation.All()
}

// CreateBackup creates a new snapshot and returns its metadata.
func (a *AdminAdapter) CreateBackup() (interface{}, error) {
	return a.backup.CreateSnapshot()
}

// ListBackups returns all available snapshots.
func (a *AdminAdapter) ListBackups() interface{} {
	return a.backup.ListSnapshots()
}

// RetentionStats returns current retention statistics.
func (a *AdminAdapter) RetentionStats() interface{} {
	return a.retention.Stats()
}
