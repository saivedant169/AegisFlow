package evidence

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// Record is a single hash-linked entry in the evidence chain.
type Record struct {
	Index        int                      `json:"index"`
	Timestamp    time.Time                `json:"timestamp"`
	Envelope     *envelope.ActionEnvelope `json:"envelope"`
	PreviousHash string                   `json:"previous_hash"`
	Hash         string                   `json:"hash"`
}

// SessionChain is a hash-linked chain of action records for a single session.
type SessionChain struct {
	mu        sync.RWMutex
	sessionID string
	records   []Record
	lastHash  string
}

func NewSessionChain(sessionID string) *SessionChain {
	return &SessionChain{
		sessionID: sessionID,
		records:   make([]Record, 0),
	}
}

func (c *SessionChain) SessionID() string {
	return c.sessionID
}

// Record adds an ActionEnvelope to the chain.
func (c *SessionChain) Record(env *envelope.ActionEnvelope) (*Record, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rec := Record{
		Index:        len(c.records),
		Timestamp:    time.Now().UTC(),
		Envelope:     env,
		PreviousHash: c.lastHash,
	}
	rec.Hash = computeRecordHash(rec)
	c.lastHash = rec.Hash
	c.records = append(c.records, rec)

	// Set evidence hash on the envelope
	env.EvidenceHash = rec.Hash

	return &rec, nil
}

// Records returns all records in order.
func (c *SessionChain) Records() []Record {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]Record, len(c.records))
	copy(result, c.records)
	return result
}

// Count returns the number of records.
func (c *SessionChain) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.records)
}

func computeRecordHash(r Record) string {
	data := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s|%s",
		r.Index,
		r.Timestamp.UTC().Format(time.RFC3339Nano),
		r.Envelope.ID,
		r.Envelope.Tool,
		r.Envelope.PolicyDecision,
		r.Envelope.Hash(),
		r.Envelope.RequestedCapability,
		r.PreviousHash,
	)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// Export returns the full chain as JSON bytes.
func (c *SessionChain) Export() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	bundle := map[string]interface{}{
		"session_id":  c.sessionID,
		"records":     c.records,
		"count":       len(c.records),
		"last_hash":   c.lastHash,
		"exported_at": time.Now().UTC(),
	}
	return json.MarshalIndent(bundle, "", "  ")
}
