package manifest

import (
	"fmt"
	"path"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// DriftType categorizes what kind of drift was detected.
type DriftType string

const (
	DriftUnexpectedTool     DriftType = "unexpected_tool"
	DriftUnexpectedResource DriftType = "unexpected_resource"
	DriftUnexpectedProtocol DriftType = "unexpected_protocol"
	DriftUnexpectedVerb     DriftType = "unexpected_verb"
	DriftExceededActions    DriftType = "exceeded_max_actions"
	DriftExceededBudget     DriftType = "exceeded_max_budget"
	DriftExpiredManifest    DriftType = "manifest_expired"
)

// DriftEvent records a single detected deviation from a manifest.
type DriftEvent struct {
	Type       DriftType `json:"type"`
	EnvelopeID string    `json:"envelope_id"`
	ManifestID string    `json:"manifest_id"`
	Tool       string    `json:"tool"`
	Protocol   string    `json:"protocol"`
	Target     string    `json:"target"`
	Verb       string    `json:"verb"`
	Message    string    `json:"message"`
	Severity   string    `json:"severity"` // warning, violation
	Timestamp  time.Time `json:"timestamp"`
}

// DriftDetector compares actual actions against a declared manifest.
type DriftDetector struct{}

// NewDriftDetector creates a new DriftDetector.
func NewDriftDetector() *DriftDetector {
	return &DriftDetector{}
}

// Check evaluates an action envelope against a manifest and returns any drift events.
// actionCount is the number of actions already taken in this session.
// currentBudget is the cumulative cost so far.
func (d *DriftDetector) Check(manifest *TaskManifest, env *envelope.ActionEnvelope, actionCount int, currentBudget float64) []DriftEvent {
	var events []DriftEvent
	now := time.Now().UTC()

	// Check manifest expiration
	if !manifest.ExpiresAt.IsZero() && now.After(manifest.ExpiresAt) {
		events = append(events, DriftEvent{
			Type:       DriftExpiredManifest,
			EnvelopeID: env.ID,
			ManifestID: manifest.ID,
			Tool:       env.Tool,
			Protocol:   string(env.Protocol),
			Target:     env.Target,
			Verb:       string(env.RequestedCapability),
			Message:    "manifest has expired",
			Severity:   "violation",
			Timestamp:  now,
		})
	}

	// Check tool against AllowedTools
	if len(manifest.AllowedTools) > 0 && !globMatchAny(manifest.AllowedTools, env.Tool) {
		events = append(events, DriftEvent{
			Type:       DriftUnexpectedTool,
			EnvelopeID: env.ID,
			ManifestID: manifest.ID,
			Tool:       env.Tool,
			Protocol:   string(env.Protocol),
			Target:     env.Target,
			Verb:       string(env.RequestedCapability),
			Message:    fmt.Sprintf("tool %q not in allowed tools %v", env.Tool, manifest.AllowedTools),
			Severity:   "violation",
			Timestamp:  now,
		})
	}

	// Check target against AllowedResources
	if len(manifest.AllowedResources) > 0 && !globMatchAny(manifest.AllowedResources, env.Target) {
		events = append(events, DriftEvent{
			Type:       DriftUnexpectedResource,
			EnvelopeID: env.ID,
			ManifestID: manifest.ID,
			Tool:       env.Tool,
			Protocol:   string(env.Protocol),
			Target:     env.Target,
			Verb:       string(env.RequestedCapability),
			Message:    fmt.Sprintf("target %q not in allowed resources %v", env.Target, manifest.AllowedResources),
			Severity:   "violation",
			Timestamp:  now,
		})
	}

	// Check protocol against AllowedProtocols
	if len(manifest.AllowedProtocols) > 0 && !stringInSlice(string(env.Protocol), manifest.AllowedProtocols) {
		events = append(events, DriftEvent{
			Type:       DriftUnexpectedProtocol,
			EnvelopeID: env.ID,
			ManifestID: manifest.ID,
			Tool:       env.Tool,
			Protocol:   string(env.Protocol),
			Target:     env.Target,
			Verb:       string(env.RequestedCapability),
			Message:    fmt.Sprintf("protocol %q not in allowed protocols %v", env.Protocol, manifest.AllowedProtocols),
			Severity:   "violation",
			Timestamp:  now,
		})
	}

	// Check verb/capability against AllowedVerbs
	if len(manifest.AllowedVerbs) > 0 && !stringInSlice(string(env.RequestedCapability), manifest.AllowedVerbs) {
		events = append(events, DriftEvent{
			Type:       DriftUnexpectedVerb,
			EnvelopeID: env.ID,
			ManifestID: manifest.ID,
			Tool:       env.Tool,
			Protocol:   string(env.Protocol),
			Target:     env.Target,
			Verb:       string(env.RequestedCapability),
			Message:    fmt.Sprintf("verb %q not in allowed verbs %v", env.RequestedCapability, manifest.AllowedVerbs),
			Severity:   "warning",
			Timestamp:  now,
		})
	}

	// Check action count
	if manifest.MaxActions > 0 && actionCount > manifest.MaxActions {
		events = append(events, DriftEvent{
			Type:       DriftExceededActions,
			EnvelopeID: env.ID,
			ManifestID: manifest.ID,
			Tool:       env.Tool,
			Protocol:   string(env.Protocol),
			Target:     env.Target,
			Verb:       string(env.RequestedCapability),
			Message:    fmt.Sprintf("action count %d exceeds max %d", actionCount, manifest.MaxActions),
			Severity:   "violation",
			Timestamp:  now,
		})
	}

	// Check budget
	if manifest.MaxBudget > 0 && currentBudget > manifest.MaxBudget {
		events = append(events, DriftEvent{
			Type:       DriftExceededBudget,
			EnvelopeID: env.ID,
			ManifestID: manifest.ID,
			Tool:       env.Tool,
			Protocol:   string(env.Protocol),
			Target:     env.Target,
			Verb:       string(env.RequestedCapability),
			Message:    fmt.Sprintf("budget %.2f exceeds max %.2f", currentBudget, manifest.MaxBudget),
			Severity:   "violation",
			Timestamp:  now,
		})
	}

	return events
}

// globMatchAny returns true if value matches any of the glob patterns.
func globMatchAny(patterns []string, value string) bool {
	for _, pattern := range patterns {
		if matched, _ := path.Match(pattern, value); matched {
			return true
		}
	}
	return false
}

// stringInSlice returns true if s is found in the slice.
func stringInSlice(s string, slice []string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
