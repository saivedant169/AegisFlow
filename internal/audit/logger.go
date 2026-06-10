package audit

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"
)

type Entry struct {
	ID           int64     `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Actor        string    `json:"actor"`
	ActorRole    string    `json:"actor_role"`
	Action       string    `json:"action"`
	Resource     string    `json:"resource"`
	Detail       string    `json:"detail"`
	TenantID     string    `json:"tenant_id"`
	Model        string    `json:"model,omitempty"`
	PreviousHash string    `json:"previous_hash"`
	EntryHash    string    `json:"entry_hash"`
}

type Logger struct {
	store    Store
	queue    chan Entry
	lastHash string
	mu       sync.Mutex
	stopCh   chan struct{}
}

type Store interface {
	Insert(entry Entry) error
	Query(filters QueryFilters) ([]Entry, error)
	LastHash() (string, error)
	Migrate() error
}

type QueryFilters struct {
	Actor     string
	ActorRole string
	Action    string
	TenantID  string
	From      time.Time
	To        time.Time
	Limit     int
}

func NewLogger(store Store) (*Logger, error) {
	if err := store.Migrate(); err != nil {
		return nil, fmt.Errorf("audit migrate: %w", err)
	}
	lastHash, err := store.LastHash()
	if err != nil {
		return nil, fmt.Errorf("audit last hash: %w", err)
	}
	l := &Logger{
		store:    store,
		queue:    make(chan Entry, 1024),
		lastHash: lastHash,
		stopCh:   make(chan struct{}),
	}
	go l.writer()
	return l, nil
}

func (l *Logger) Log(actor, actorRole, action, resource, detail, tenantID, model string) {
	entry := Entry{
		Timestamp: time.Now(),
		Actor:     actor,
		ActorRole: actorRole,
		Action:    action,
		Resource:  resource,
		Detail:    detail,
		TenantID:  tenantID,
		Model:     model,
	}
	select {
	case l.queue <- entry:
	default:
		log.Printf("audit: queue full, dropping entry: %s %s", action, resource)
	}
}

func (l *Logger) writer() {
	for {
		select {
		case <-l.stopCh:
			return
		case entry := <-l.queue:
			l.mu.Lock()
			entry.PreviousHash = l.lastHash
			entry.EntryHash = computeHash(entry)
			l.lastHash = entry.EntryHash
			l.mu.Unlock()

			if err := l.store.Insert(entry); err != nil {
				log.Printf("audit: failed to insert: %v", err)
			}
		}
	}
}

func (l *Logger) Stop() {
	close(l.stopCh)
}

func (l *Logger) Query(filters QueryFilters) ([]Entry, error) {
	return l.store.Query(filters)
}

func computeHash(e Entry) string {
	// Length-prefix every field so the boundaries are unambiguous. The old
	// "%s|%s|..." join let a '|' inside a field (e.g. attacker-controlled
	// Detail/Actor) shift the boundaries, so two different entries could hash
	// to the same value and a forged record could still pass Verify.
	return canonicalHash(
		e.Timestamp.UTC().Format(time.RFC3339Nano),
		e.Actor, e.ActorRole, e.Action, e.Resource, e.Detail, e.TenantID, e.PreviousHash,
	)
}

// canonicalHash hashes an injective encoding of its fields: each field is
// written as a 4-byte big-endian length followed by its bytes, so no field's
// contents can be mistaken for a delimiter or shift another field's boundary.
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
