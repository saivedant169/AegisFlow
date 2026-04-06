package toolpolicy

import (
	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// AdminAdapter wraps a toolpolicy.Engine to satisfy the admin.ToolPolicyProvider interface.
type AdminAdapter struct {
	engine *Engine
}

// NewAdminAdapter creates a new AdminAdapter wrapping the given Engine.
func NewAdminAdapter(e *Engine) *AdminAdapter {
	return &AdminAdapter{engine: e}
}

// Evaluate evaluates an ActionEnvelope against the tool policy engine and returns
// the decision as a string ("allow", "review", or "block").
func (a *AdminAdapter) Evaluate(env *envelope.ActionEnvelope) string {
	return string(a.engine.Evaluate(env))
}

// EvaluateWithTrace evaluates an ActionEnvelope and returns the full decision
// trace as a JSON-serializable value.
func (a *AdminAdapter) EvaluateWithTrace(env *envelope.ActionEnvelope) interface{} {
	return a.engine.EvaluateWithTrace(env)
}
