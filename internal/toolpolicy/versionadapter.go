package toolpolicy

// VersionAdminAdapter bridges the PolicyVersionStore and Engine to the
// admin.PolicyVersionProvider interface.
type VersionAdminAdapter struct {
	store  *PolicyVersionStore
	engine *Engine
}

// NewVersionAdminAdapter creates a new adapter for the admin API.
func NewVersionAdminAdapter(store *PolicyVersionStore, engine *Engine) *VersionAdminAdapter {
	return &VersionAdminAdapter{store: store, engine: engine}
}

// ListVersions returns all recorded policy versions.
func (a *VersionAdminAdapter) ListVersions() interface{} {
	return a.store.List()
}

// GetVersion returns a specific policy version by number.
func (a *VersionAdminAdapter) GetVersion(version int) (interface{}, error) {
	return a.store.Get(version)
}

// CurrentVersion returns the most recent policy version.
func (a *VersionAdminAdapter) CurrentVersion() interface{} {
	return a.store.Current()
}

// Rollback reverts the engine to a previous policy version and records a new
// snapshot with source "rollback".
func (a *VersionAdminAdapter) Rollback(version int) error {
	v, err := a.store.Get(version)
	if err != nil {
		return err
	}
	a.engine.ReplaceRules(v.Rules, v.DefaultDecision)
	a.store.Snapshot(v.Rules, v.DefaultDecision, "rollback")
	return nil
}
