package manifest

import (
	"fmt"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// AdminAdapter wraps manifest Store and DriftDetector to satisfy the
// admin.ManifestProvider interface.
type AdminAdapter struct {
	store    *Store
	detector *DriftDetector
}

// NewAdminAdapter creates a new AdminAdapter.
func NewAdminAdapter(store *Store, detector *DriftDetector) *AdminAdapter {
	return &AdminAdapter{store: store, detector: detector}
}

// Register registers a new manifest. Accepts either *TaskManifest or map[string]interface{}.
func (a *AdminAdapter) Register(m interface{}) error {
	switch v := m.(type) {
	case *TaskManifest:
		return a.store.Register(v)
	case map[string]interface{}:
		manifest := &TaskManifest{}
		if id, ok := v["id"].(string); ok {
			manifest.ID = id
		}
		if taskID, ok := v["task_id"].(string); ok {
			manifest.TaskID = taskID
		}
		if desc, ok := v["description"].(string); ok {
			manifest.Description = desc
		}
		if owner, ok := v["owner"].(string); ok {
			manifest.Owner = owner
		}
		if exp, ok := v["expires_at"].(time.Time); ok {
			manifest.ExpiresAt = exp
		}
		if tools, ok := v["allowed_tools"].([]string); ok {
			manifest.AllowedTools = tools
		}
		if resources, ok := v["allowed_resources"].([]string); ok {
			manifest.AllowedResources = resources
		}
		if protocols, ok := v["allowed_protocols"].([]string); ok {
			manifest.AllowedProtocols = protocols
		}
		if verbs, ok := v["allowed_verbs"].([]string); ok {
			manifest.AllowedVerbs = verbs
		}
		if maxActions, ok := v["max_actions"].(int); ok {
			manifest.MaxActions = maxActions
		}
		if maxBudget, ok := v["max_budget"].(float64); ok {
			manifest.MaxBudget = maxBudget
		}
		if riskTier, ok := v["risk_tier"].(string); ok {
			manifest.RiskTier = riskTier
		}
		return a.store.Register(manifest)
	default:
		return fmt.Errorf("unsupported manifest type: %T", m)
	}
}

// Get returns a manifest by ID.
func (a *AdminAdapter) Get(id string) (interface{}, error) {
	return a.store.Get(id)
}

// List returns all active manifests.
func (a *AdminAdapter) List() interface{} {
	return a.store.ListActive()
}

// Deactivate deactivates a manifest.
func (a *AdminAdapter) Deactivate(id string) error {
	return a.store.Deactivate(id)
}

// GetDrift returns drift events for a manifest.
func (a *AdminAdapter) GetDrift(id string) interface{} {
	return a.store.GetDrift(id)
}

// CheckDrift checks an envelope against the manifest for a given task and records any drift.
func (a *AdminAdapter) CheckDrift(taskID string, env *envelope.ActionEnvelope, actionCount int, currentBudget float64) interface{} {
	m, err := a.store.GetByTaskID(taskID)
	if err != nil {
		return nil
	}
	events := a.detector.Check(m, env, actionCount, currentBudget)
	a.store.RecordDrift(m.ID, events)
	if len(events) == 0 {
		return nil
	}
	return events
}

// Store returns the underlying store for direct access.
func (a *AdminAdapter) Store() *Store {
	return a.store
}
