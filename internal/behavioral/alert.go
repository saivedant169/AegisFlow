package behavioral

import "time"

// BehaviorAlert represents a detected risky behavioral pattern in a session.
type BehaviorAlert struct {
	Rule      string    `json:"rule"`
	Severity  string    `json:"severity"` // "warning", "critical"
	Message   string    `json:"message"`
	Actions   []string  `json:"actions"`    // envelope IDs involved
	RiskScore int       `json:"risk_score"` // contribution to session risk
	Timestamp time.Time `json:"timestamp"`
}
