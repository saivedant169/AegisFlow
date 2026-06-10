package evidence

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"strconv"
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
	// Length-prefix each field so its contents can't shift the boundaries; the
	// old "%d|%s|..." join was ambiguous when a field contained '|'.
	return canonicalHash(
		strconv.Itoa(r.Index),
		r.Timestamp.UTC().Format(time.RFC3339Nano),
		r.Envelope.ID,
		r.Envelope.Tool,
		string(r.Envelope.PolicyDecision),
		r.Envelope.Hash(),
		string(r.Envelope.RequestedCapability),
		r.PreviousHash,
	)
}

// canonicalHash hashes an injective encoding of its fields: each is written as
// a 4-byte big-endian length followed by its bytes, so no field can be mistaken
// for a delimiter or shift another's boundary.
func canonicalHash(fields ...string) string {
	h := sha256.New()
	var lenBuf [4]byte
	for _, f := range fields {
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(f)))
		h.Write(lenBuf[:])
		h.Write([]byte(f))
	}
	return hex.EncodeToString(h.Sum(nil))
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
