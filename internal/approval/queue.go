package approval

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

// Notifier receives approval lifecycle events.
type Notifier interface {
	NotifyReview(item *ApprovalItem) error
	NotifyApproved(item *ApprovalItem) error
	NotifyDenied(item *ApprovalItem) error
}

const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusDenied   = "denied"
	StatusExpired  = "expired"
)

// ApprovalItem wraps an ActionEnvelope with review metadata.
type ApprovalItem struct {
	ID            string                   `json:"id"`
	Envelope      *envelope.ActionEnvelope `json:"envelope"`
	Status        string                   `json:"status"`
	SubmittedAt   time.Time                `json:"submitted_at"`
	ExpireAt      time.Time                `json:"expire_at"`
	ReviewedAt    *time.Time               `json:"reviewed_at,omitempty"`
	Reviewer      string                   `json:"reviewer,omitempty"`
	ReviewComment string                   `json:"review_comment,omitempty"`
	consumed      bool                     // set once the approval has been used
}

// Queue manages pending approval items.
type Queue struct {
	mu        sync.RWMutex
	pending   map[string]*ApprovalItem
	history   []*ApprovalItem
	maxSize   int
	Timeout   time.Duration
	notifiers []Notifier
	store     approvalStore
}

func NewQueue(maxSize int) *Queue {
	return &Queue{
		pending: make(map[string]*ApprovalItem),
		history: make([]*ApprovalItem, 0),
		maxSize: maxSize,
		Timeout: 30 * time.Minute, // default timeout
	}
}

// NewPersistentQueue restores pending and recent approval items from SQLite.
func NewPersistentQueue(maxSize int, db *sql.DB, signingKey []byte) (*Queue, error) {
	store, err := newSQLiteApprovalStore(db, signingKey)
	if err != nil {
		return nil, err
	}
	pending, history, err := store.Load(1000)
	if err != nil {
		return nil, fmt.Errorf("restore approval state: %w", err)
	}
	q := &Queue{
		pending: make(map[string]*ApprovalItem, len(pending)),
		history: history,
		maxSize: maxSize,
		Timeout: 30 * time.Minute,
		store:   store,
	}
	for _, item := range pending {
		q.pending[item.ID] = item
	}
	// Legacy MCP approvals lack trusted identity and must be reviewed again.
	items := append(append([]*ApprovalItem{}, history...), pending...)
	for _, item := range items {
		actor := item.Envelope.Actor
		if item.Envelope.Protocol == envelope.ProtocolMCP && (actor.TenantID == "" || actor.SessionID == "" || !strings.HasPrefix(actor.ID, "principal-v1-")) && !item.consumed && (item.Status == StatusPending || item.Status == StatusApproved) {
			item.Status = StatusExpired
			if err := store.Save(item); err != nil {
				return nil, fmt.Errorf("expire legacy approval: %w", err)
			}
			if _, ok := q.pending[item.ID]; ok {
				delete(q.pending, item.ID)
				q.history = append(q.history, item)
			}
		}
	}
	return q, nil
}

// AddNotifier registers a notifier to receive approval lifecycle events.
func (q *Queue) AddNotifier(n Notifier) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.notifiers = append(q.notifiers, n)
}

func (q *Queue) notify(item *ApprovalItem, fn func(Notifier, *ApprovalItem) error) {
	for _, n := range q.notifiers {
		if err := fn(n, item); err != nil {
			log.Printf("[approval] notifier error: %v", err)
		}
	}
}

// Submit adds an ActionEnvelope to the approval queue. Returns the approval ID.
func (q *Queue) Submit(env *envelope.ActionEnvelope) (string, error) {
	snapshot, err := cloneApprovalEnvelope(env)
	if err != nil {
		return "", err
	}
	q.mu.Lock()

	if len(q.pending) >= q.maxSize {
		q.mu.Unlock()
		return "", errors.New("approval queue is full")
	}
	if _, exists := q.pending[snapshot.ID]; exists {
		q.mu.Unlock()
		return "", errors.New("approval item already exists: " + snapshot.ID)
	}
	for _, existing := range q.history {
		if existing.ID == snapshot.ID {
			q.mu.Unlock()
			return "", errors.New("approval item already exists: " + snapshot.ID)
		}
	}

	now := time.Now().UTC()
	item := &ApprovalItem{
		ID:          snapshot.ID,
		Envelope:    snapshot,
		Status:      StatusPending,
		SubmittedAt: now,
		ExpireAt:    now.Add(q.Timeout),
	}
	if q.store != nil {
		if err := q.store.Save(item); err != nil {
			q.mu.Unlock()
			return "", err
		}
	}
	q.pending[item.ID] = item
	q.mu.Unlock()

	q.notify(item, Notifier.NotifyReview)
	return item.ID, nil
}

func cloneApprovalEnvelope(env *envelope.ActionEnvelope) (*envelope.ActionEnvelope, error) {
	if env == nil {
		return nil, errors.New("approval envelope is required")
	}
	data, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("copy approval envelope: %w", err)
	}
	var snapshot envelope.ActionEnvelope
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("copy approval envelope: %w", err)
	}
	return &snapshot, nil
}

// Pending returns all pending items.
func (q *Queue) Pending() []*ApprovalItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	items := make([]*ApprovalItem, 0, len(q.pending))
	for _, item := range q.pending {
		items = append(items, snapshotItem(item))
	}
	return items
}

// Get returns an item by ID (pending or history).
func (q *Queue) Get(id string) (*ApprovalItem, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if item, ok := q.pending[id]; ok {
		return snapshotItem(item), nil
	}
	for _, item := range q.history {
		if item.ID == id {
			return snapshotItem(item), nil
		}
	}
	return nil, errors.New("approval item not found: " + id)
}

// Approve marks an item as approved.
func (q *Queue) Approve(id, reviewer, comment string) (*ApprovalItem, error) {
	return q.resolve(id, StatusApproved, reviewer, comment)
}

// Deny marks an item as denied.
func (q *Queue) Deny(id, reviewer, comment string) (*ApprovalItem, error) {
	return q.resolve(id, StatusDenied, reviewer, comment)
}

func (q *Queue) resolve(id, status, reviewer, comment string) (*ApprovalItem, error) {
	q.mu.Lock()

	item, ok := q.pending[id]
	if !ok {
		q.mu.Unlock()
		return nil, errors.New("approval item not found or already reviewed: " + id)
	}

	now := time.Now().UTC()
	if !now.Before(item.ExpireAt) {
		q.mu.Unlock()
		return nil, errors.New("approval item expired: " + id)
	}
	updated := *item
	updated.Status = status
	updated.ReviewedAt = &now
	updated.Reviewer = reviewer
	updated.ReviewComment = comment
	if q.store != nil {
		if err := q.store.Save(&updated); err != nil {
			q.mu.Unlock()
			return nil, err
		}
	}
	*item = updated

	delete(q.pending, id)
	q.history = append(q.history, item)

	// Cap history at 1000
	if len(q.history) > 1000 {
		q.history = q.history[len(q.history)-1000:]
	}
	if q.store != nil {
		if err := q.store.PruneHistory(1000); err != nil {
			log.Printf("[approval] persistent history cleanup failed: %v", err)
		}
	}

	q.mu.Unlock()

	switch status {
	case StatusApproved:
		q.notify(item, Notifier.NotifyApproved)
	case StatusDenied:
		q.notify(item, Notifier.NotifyDenied)
	}

	return item, nil
}

// IsApprovedForTool checks if there's a recently approved item for the given tool name.
// Used by the MCP gateway to allow retries after approval.
func (q *Queue) IsApprovedForTool(tool string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	for _, item := range q.history {
		if item.Status == StatusApproved && item.Envelope != nil && item.Envelope.Tool == tool {
			return true
		}
	}
	return false
}

// ConsumeApprovalForEnvelope returns true at most once for an approved action
// whose fingerprint covers actor, task, tool, target, arguments, and capability
// matches env and that was approved within the queue timeout. Matching on the
// fingerprint rather than the tool name stops a single approval from covering
// every other call of the same tool with different arguments, and consuming the
// item makes each approval good for exactly one execution.
func (q *Queue) ConsumeApprovalForEnvelope(env *envelope.ActionEnvelope) bool {
	if env == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	fingerprint := env.ApprovalFingerprint()
	for _, item := range q.history {
		if item.Status == StatusApproved && !item.consumed && item.Envelope != nil &&
			item.Envelope.ApprovalFingerprint() == fingerprint &&
			item.ReviewedAt != nil && now.Sub(*item.ReviewedAt) <= q.Timeout {
			if q.store != nil {
				consumed, err := q.store.Consume(item)
				if err != nil {
					log.Printf("[approval] persistent consume failed: %v", err)
					return false
				}
				if !consumed {
					item.consumed = true
					continue
				}
			}
			item.consumed = true
			return true
		}
	}
	return false
}

// CleanupExpired auto-denies items that have exceeded their expiration time.
// Returns the number of items expired.
func (q *Queue) CleanupExpired() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now().UTC()
	expired := 0
	for id, item := range q.pending {
		if now.After(item.ExpireAt) {
			updated := *item
			updated.Status = StatusExpired
			updated.ReviewedAt = &now
			updated.Reviewer = "system"
			updated.ReviewComment = "auto-denied: approval timeout exceeded"
			if q.store != nil {
				if err := q.store.Save(&updated); err != nil {
					log.Printf("[approval] persistent expiry failed for %s: %v", id, err)
					continue
				}
			}
			*item = updated
			delete(q.pending, id)
			q.history = append(q.history, item)
			expired++
		}
	}

	// Cap history at 1000
	if len(q.history) > 1000 {
		q.history = q.history[len(q.history)-1000:]
	}
	if q.store != nil {
		if err := q.store.PruneHistory(1000); err != nil {
			log.Printf("[approval] persistent history cleanup failed: %v", err)
		}
	}

	return expired
}

// History returns the most recent N resolved items.
func (q *Queue) History(limit int) []*ApprovalItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if limit <= 0 || limit > len(q.history) {
		limit = len(q.history)
	}
	start := len(q.history) - limit
	result := make([]*ApprovalItem, limit)
	for i, item := range q.history[start:] {
		result[i] = snapshotItem(item)
	}
	return result
}

func snapshotItem(item *ApprovalItem) *ApprovalItem {
	copy := *item
	copy.Envelope, _ = cloneApprovalEnvelope(item.Envelope)
	if item.ReviewedAt != nil {
		reviewed := *item.ReviewedAt
		copy.ReviewedAt = &reviewed
	}
	return &copy
}
