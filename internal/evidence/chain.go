package evidence

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"sync"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// Record is a single hash-linked entry in the evidence chain. Hash is the
// content hash (anyone can recompute it to check the chain links). Signature,
// when present, is an HMAC of Hash under the chain's secret key — only a
// key-holder can produce or verify it, which is what makes a record
// tamper-evident against someone who only has read/write access to the store.
type Record struct {
	Index        int                      `json:"index"`
	Timestamp    time.Time                `json:"timestamp"`
	Envelope     *envelope.ActionEnvelope `json:"envelope"`
	PreviousHash string                   `json:"previous_hash"`
	Hash         string                   `json:"hash"`
	Signature    string                   `json:"signature,omitempty"`
}

// SessionChain is a hash-linked chain of action records for a single session.
type SessionChain struct {
	mu        sync.RWMutex
	sessionID string
	records   []Record
	lastHash  string
	key       []byte // optional HMAC key; when set, records are signed
	append    func(Record) error
}

func NewSessionChain(sessionID string) *SessionChain {
	return &SessionChain{
		sessionID: sessionID,
		records:   make([]Record, 0),
	}
}

// NewSignedSessionChain is like NewSessionChain but signs each record with an
// HMAC under key, so the chain can't be silently rewritten by anyone who
// doesn't hold the key.
func NewSignedSessionChain(sessionID string, key []byte) *SessionChain {
	return &SessionChain{
		sessionID: sessionID,
		records:   make([]Record, 0),
		key:       key,
	}
}

func restoreSessionChain(sessionID string, key []byte, records []Record, appendRecord func(Record) error) (*SessionChain, error) {
	for i := range records {
		if records[i].Index != i {
			return nil, fmt.Errorf("record index %d found at position %d", records[i].Index, i)
		}
		if records[i].Envelope == nil {
			return nil, fmt.Errorf("record %d has no envelope", i)
		}
		recordSessionID := records[i].Envelope.Actor.SessionID
		if recordSessionID == "" {
			recordSessionID = defaultSessionFallback
		}
		if recordSessionID != sessionID {
			return nil, fmt.Errorf("record %d belongs to session %q, not %q", i, recordSessionID, sessionID)
		}
	}

	var result VerifyResult
	if len(key) > 0 {
		result = VerifySignatures(records, key)
	} else {
		result = Verify(records)
	}
	if !result.Valid {
		return nil, errors.New(result.Message)
	}

	chain := &SessionChain{
		sessionID: sessionID,
		records:   append([]Record(nil), records...),
		key:       append([]byte(nil), key...),
		append:    appendRecord,
	}
	if len(records) > 0 {
		chain.lastHash = records[len(records)-1].Hash
	}
	return chain, nil
}

func (c *SessionChain) SessionID() string {
	return c.sessionID
}

// Record adds an ActionEnvelope to the chain.
func (c *SessionChain) Record(env *envelope.ActionEnvelope) (*Record, error) {
	if env == nil {
		return nil, errors.New("evidence envelope is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	// A session cannot change tenant while scoped readers export it.
	if len(c.records) > 0 && c.records[0].Envelope.Actor.TenantID != env.Actor.TenantID {
		return nil, errors.New("evidence session belongs to another tenant")
	}

	snapshot, err := cloneEnvelope(env)
	if err != nil {
		return nil, fmt.Errorf("copy evidence envelope: %w", err)
	}

	rec := Record{
		Index:        len(c.records),
		Timestamp:    time.Now().UTC(),
		Envelope:     snapshot,
		PreviousHash: c.lastHash,
	}
	rec.Hash = computeRecordHash(rec)
	if len(c.key) > 0 {
		rec.Signature = signHash(c.key, rec.Hash)
	}
	rec.Envelope.EvidenceHash = rec.Hash
	if c.append != nil {
		if err := c.append(rec); err != nil {
			return nil, fmt.Errorf("persist evidence record: %w", err)
		}
	}
	c.lastHash = rec.Hash
	c.records = append(c.records, rec)

	// Set evidence hash on the envelope
	env.EvidenceHash = rec.Hash

	return &rec, nil
}

func cloneEnvelope(env *envelope.ActionEnvelope) (*envelope.ActionEnvelope, error) {
	snapshot := *env
	parameters, err := cloneParameters(env.Parameters)
	if err != nil {
		return nil, err
	}
	snapshot.Parameters = parameters
	if env.Resource != nil {
		resourceCopy := *env.Resource
		resourceCopy.Path = append([]string(nil), env.Resource.Path...)
		if env.Resource.Properties != nil {
			resourceCopy.Properties = make(map[string]string, len(env.Resource.Properties))
			maps.Copy(resourceCopy.Properties, env.Resource.Properties)
		}
		snapshot.Resource = &resourceCopy
	}
	if env.Result != nil {
		resultCopy := *env.Result
		snapshot.Result = &resultCopy
	}
	return &snapshot, nil
}

func cloneParameters(parameters map[string]any) (map[string]any, error) {
	if parameters == nil {
		return nil, nil
	}
	cloned := make(map[string]any, len(parameters))
	for key, value := range parameters {
		copy, err := cloneParameter(value)
		if err != nil {
			return nil, fmt.Errorf("parameter %q: %w", key, err)
		}
		cloned[key] = copy
	}
	return cloned, nil
}

func cloneParameter(value any) (any, error) {
	switch typed := value.(type) {
	case nil, bool, string,
		float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		json.Number:
		return typed, nil
	case []byte:
		return append([]byte(nil), typed...), nil
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			copy, err := cloneParameter(item)
			if err != nil {
				return nil, err
			}
			cloned[index] = copy
		}
		return cloned, nil
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		maps.Copy(cloned, typed)
		return cloned, nil
	case map[string]any:
		return cloneParameters(typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		var cloned any
		if err := json.Unmarshal(data, &cloned); err != nil {
			return nil, err
		}
		return cloned, nil
	}
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

	bundle := map[string]any{
		"session_id":  c.sessionID,
		"records":     c.records,
		"count":       len(c.records),
		"last_hash":   c.lastHash,
		"exported_at": time.Now().UTC(),
	}
	return json.MarshalIndent(bundle, "", "  ")
}

// signHash returns the hex HMAC-SHA256 of a record hash under key.
func signHash(key []byte, hash string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(hash))
	return hex.EncodeToString(mac.Sum(nil))
}
