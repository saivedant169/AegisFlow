package evidence

import "time"

// SessionManifest summarizes a session's evidence for export.
type SessionManifest struct {
	SessionID    string    `json:"session_id"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	TotalActions int       `json:"total_actions"`
	Allowed      int       `json:"allowed"`
	Reviewed     int       `json:"reviewed"`
	Blocked      int       `json:"blocked"`
	FirstHash    string    `json:"first_hash"`
	LastHash     string    `json:"last_hash"`
	ChainValid   bool      `json:"chain_valid"`
}

// Manifest builds a SessionManifest from the chain.
func (c *SessionChain) Manifest() SessionManifest {
	c.mu.RLock()
	defer c.mu.RUnlock()

	m := SessionManifest{
		SessionID:    c.sessionID,
		TotalActions: len(c.records),
		LastHash:     c.lastHash,
	}

	if len(c.records) > 0 {
		m.StartedAt = c.records[0].Timestamp
		m.EndedAt = c.records[len(c.records)-1].Timestamp
		m.FirstHash = c.records[0].Hash
	}

	for _, r := range c.records {
		switch r.Envelope.PolicyDecision {
		case "allow":
			m.Allowed++
		case "review":
			m.Reviewed++
		case "block":
			m.Blocked++
		}
	}

	result := Verify(c.records)
	m.ChainValid = result.Valid

	return m
}
