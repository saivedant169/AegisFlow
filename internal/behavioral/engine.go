package behavioral

import (
	"sync"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// SessionAnalyzer tracks action history per session and runs behavioral
// detection rules over action sequences to detect risky patterns.
type SessionAnalyzer struct {
	mu             sync.RWMutex
	sessionID      string
	history        []envelope.ActionEnvelope
	alerts         []BehaviorAlert
	rules          []Rule
	killSwitchScore int
	windowMinutes  int
	blocked        bool
}

// NewSessionAnalyzer creates a new analyzer for a given session.
// killSwitchScore is the cumulative risk score threshold that triggers auto-block (0 = disabled).
// windowMinutes is the analysis time window in minutes (0 = unlimited).
func NewSessionAnalyzer(sessionID string, rules []Rule, killSwitchScore, windowMinutes int) *SessionAnalyzer {
	if rules == nil {
		rules = DefaultRules()
	}
	return &SessionAnalyzer{
		sessionID:       sessionID,
		rules:           rules,
		killSwitchScore: killSwitchScore,
		windowMinutes:   windowMinutes,
	}
}

// RecordAction adds an action envelope to the session history.
func (sa *SessionAnalyzer) RecordAction(env *envelope.ActionEnvelope) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	sa.history = append(sa.history, *env)
}

// Analyze runs all detection rules over the session history within the
// configured time window and returns any new alerts.
func (sa *SessionAnalyzer) Analyze() []BehaviorAlert {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	window := sa.windowedHistory()
	var newAlerts []BehaviorAlert

	for _, rule := range sa.rules {
		alert := rule.Detect(window)
		if alert != nil {
			if !sa.hasDuplicateAlert(alert) {
				sa.alerts = append(sa.alerts, *alert)
				newAlerts = append(newAlerts, *alert)
			}
		}
	}

	// Check kill switch.
	if sa.killSwitchScore > 0 && sa.cumulativeScore() >= sa.killSwitchScore {
		sa.blocked = true
	}

	return newAlerts
}

// SessionRiskScore returns the cumulative risk score (0-100) across all alerts.
func (sa *SessionAnalyzer) SessionRiskScore() int {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	score := sa.cumulativeScore()
	if score > 100 {
		return 100
	}
	return score
}

// Blocked returns true if the session has been auto-blocked by the kill switch.
func (sa *SessionAnalyzer) Blocked() bool {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	return sa.blocked
}

// Alerts returns all accumulated alerts.
func (sa *SessionAnalyzer) Alerts() []BehaviorAlert {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	result := make([]BehaviorAlert, len(sa.alerts))
	copy(result, sa.alerts)
	return result
}

// SessionID returns the session identifier.
func (sa *SessionAnalyzer) SessionID() string {
	return sa.sessionID
}

// History returns a copy of the action history.
func (sa *SessionAnalyzer) History() []envelope.ActionEnvelope {
	sa.mu.RLock()
	defer sa.mu.RUnlock()
	result := make([]envelope.ActionEnvelope, len(sa.history))
	copy(result, sa.history)
	return result
}

// windowedHistory returns actions within the configured time window.
// Must be called under lock.
func (sa *SessionAnalyzer) windowedHistory() []envelope.ActionEnvelope {
	if sa.windowMinutes <= 0 || len(sa.history) == 0 {
		return sa.history
	}

	cutoff := time.Now().UTC().Add(-time.Duration(sa.windowMinutes) * time.Minute)
	var result []envelope.ActionEnvelope
	for _, env := range sa.history {
		if !env.Timestamp.Before(cutoff) {
			result = append(result, env)
		}
	}
	return result
}

// cumulativeScore sums risk scores across all alerts. Must be called under lock.
func (sa *SessionAnalyzer) cumulativeScore() int {
	total := 0
	for _, a := range sa.alerts {
		total += a.RiskScore
	}
	return total
}

// hasDuplicateAlert checks if an alert with the same rule and same action set
// already exists. Must be called under lock.
func (sa *SessionAnalyzer) hasDuplicateAlert(alert *BehaviorAlert) bool {
	for _, existing := range sa.alerts {
		if existing.Rule == alert.Rule && sameActions(existing.Actions, alert.Actions) {
			return true
		}
	}
	return false
}

func sameActions(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Registry manages SessionAnalyzers for multiple sessions.
type Registry struct {
	mu              sync.RWMutex
	sessions        map[string]*SessionAnalyzer
	rules           []Rule
	killSwitchScore int
	windowMinutes   int
}

// NewRegistry creates a behavioral analysis registry.
func NewRegistry(rules []Rule, killSwitchScore, windowMinutes int) *Registry {
	if rules == nil {
		rules = DefaultRules()
	}
	return &Registry{
		sessions:        make(map[string]*SessionAnalyzer),
		rules:           rules,
		killSwitchScore: killSwitchScore,
		windowMinutes:   windowMinutes,
	}
}

// GetOrCreate returns the analyzer for a session, creating one if needed.
func (r *Registry) GetOrCreate(sessionID string) *SessionAnalyzer {
	r.mu.Lock()
	defer r.mu.Unlock()
	if sa, ok := r.sessions[sessionID]; ok {
		return sa
	}
	sa := NewSessionAnalyzer(sessionID, r.rules, r.killSwitchScore, r.windowMinutes)
	r.sessions[sessionID] = sa
	return sa
}

// Get returns the analyzer for a session, or nil if not found.
func (r *Registry) Get(sessionID string) *SessionAnalyzer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessions[sessionID]
}

// ListSessions returns all tracked session IDs.
func (r *Registry) ListSessions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.sessions))
	for id := range r.sessions {
		ids = append(ids, id)
	}
	return ids
}
