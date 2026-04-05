package costopt

// AdminAdapter bridges costopt to the admin API.
type AdminAdapter struct {
	engine  *Engine
	usageFn func() []UsageSnapshot
}

func NewAdminAdapter(engine *Engine, usageFn func() []UsageSnapshot) *AdminAdapter {
	return &AdminAdapter{engine: engine, usageFn: usageFn}
}

func (a *AdminAdapter) Recommendations() interface{} {
	snapshots := a.usageFn()
	return a.engine.Analyze(snapshots)
}
