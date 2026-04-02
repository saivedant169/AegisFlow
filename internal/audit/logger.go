package audit

import (
	"crypto/sha256"
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
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s",
		e.Timestamp.UTC().Format(time.RFC3339Nano),
		e.Actor, e.ActorRole, e.Action, e.Resource, e.Detail, e.TenantID, e.PreviousHash)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}
