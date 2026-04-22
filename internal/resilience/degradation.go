package resilience

import "sync"

// DegradationMode describes the current operating mode for a component and
// the safety impact of running in that mode.
type DegradationMode struct {
	Component    string `json:"component"`
	Mode         string `json:"mode"`          // normal, degraded, emergency
	Behavior     string `json:"behavior"`      // description of what changes
	SafetyImpact string `json:"safety_impact"` // what safety guarantees are affected
}

// DegradationManager tracks the degradation state of each component.
type DegradationManager struct {
	mu    sync.RWMutex
	modes map[string]*DegradationMode
}

// NewDegradationManager returns a DegradationManager pre-loaded with
// built-in degradation definitions for core AegisFlow components.
func NewDegradationManager() *DegradationManager {
	dm := &DegradationManager{
		modes: make(map[string]*DegradationMode),
	}
	// Register built-in components with their default (normal) modes.
	dm.modes["PolicyEngine"] = &DegradationMode{
		Component:    "PolicyEngine",
		Mode:         "normal",
		Behavior:     "all policies evaluated normally",
		SafetyImpact: "none",
	}
	dm.modes["EvidenceChain"] = &DegradationMode{
		Component:    "EvidenceChain",
		Mode:         "normal",
		Behavior:     "all actions recorded to chain",
		SafetyImpact: "none",
	}
	dm.modes["ApprovalQueue"] = &DegradationMode{
		Component:    "ApprovalQueue",
		Mode:         "normal",
		Behavior:     "approvals processed normally",
		SafetyImpact: "none",
	}
	dm.modes["CredentialBroker"] = &DegradationMode{
		Component:    "CredentialBroker",
		Mode:         "normal",
		Behavior:     "credentials issued normally",
		SafetyImpact: "none",
	}
	dm.modes["AuditLog"] = &DegradationMode{
		Component:    "AuditLog",
		Mode:         "normal",
		Behavior:     "all entries persisted immediately",
		SafetyImpact: "none",
	}
	return dm
}

// SetDegraded transitions a component into degraded mode. The built-in
// fail-safe behaviors are applied automatically:
//   - PolicyEngine: fail-closed (block all), log error
//   - EvidenceChain: buffer in memory, alert operator, continue with reduced proof
//   - ApprovalQueue: auto-deny all review actions (safe default)
//   - CredentialBroker: deny credential issuance, block actions requiring credentials
//   - AuditLog: buffer in memory, alert, flush when recovered
func (dm *DegradationManager) SetDegraded(component string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	m, ok := dm.modes[component]
	if !ok {
		dm.modes[component] = &DegradationMode{
			Component:    component,
			Mode:         "degraded",
			Behavior:     "unknown component in degraded mode",
			SafetyImpact: "unknown",
		}
		return
	}

	m.Mode = "degraded"
	switch component {
	case "PolicyEngine":
		m.Behavior = "fail-closed: all requests blocked"
		m.SafetyImpact = "no requests processed until recovery"
	case "EvidenceChain":
		m.Behavior = "buffering in memory, reduced proof guarantees"
		m.SafetyImpact = "evidence chain may have gaps if process crashes"
	case "ApprovalQueue":
		m.Behavior = "auto-deny all review actions"
		m.SafetyImpact = "no approvals possible until recovery"
	case "CredentialBroker":
		m.Behavior = "deny all credential issuance"
		m.SafetyImpact = "actions requiring credentials will fail"
	case "AuditLog":
		m.Behavior = "buffering in memory, flush on recovery"
		m.SafetyImpact = "audit entries may be lost if process crashes"
	}
}

// SetEmergency transitions a component into emergency mode.
func (dm *DegradationManager) SetEmergency(component string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	m, ok := dm.modes[component]
	if !ok {
		dm.modes[component] = &DegradationMode{
			Component:    component,
			Mode:         "emergency",
			Behavior:     "unknown component in emergency mode",
			SafetyImpact: "unknown",
		}
		return
	}
	m.Mode = "emergency"
	m.Behavior = "component fully offline, safe defaults active"
	m.SafetyImpact = "full functionality loss for " + component
}

// SetNormal transitions a component back to normal mode.
func (dm *DegradationManager) SetNormal(component string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	m, ok := dm.modes[component]
	if !ok {
		return
	}
	m.Mode = "normal"
	switch component {
	case "PolicyEngine":
		m.Behavior = "all policies evaluated normally"
	case "EvidenceChain":
		m.Behavior = "all actions recorded to chain"
	case "ApprovalQueue":
		m.Behavior = "approvals processed normally"
	case "CredentialBroker":
		m.Behavior = "credentials issued normally"
	case "AuditLog":
		m.Behavior = "all entries persisted immediately"
	default:
		m.Behavior = "operating normally"
	}
	m.SafetyImpact = "none"
}

// All returns the current degradation state for every tracked component.
func (dm *DegradationManager) All() []DegradationMode {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	out := make([]DegradationMode, 0, len(dm.modes))
	for _, m := range dm.modes {
		out = append(out, *m)
	}
	return out
}

// Get returns the degradation mode for a specific component.
func (dm *DegradationManager) Get(component string) *DegradationMode {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	m, ok := dm.modes[component]
	if !ok {
		return nil
	}
	cp := *m
	return &cp
}
