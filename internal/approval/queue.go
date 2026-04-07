package approval

import (
	"errors"
	"sync"
	"time"

	"github.com/saivedant169/AegisFlow/internal/envelope"
)

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
}

// Queue manages pending approval items.
type Queue struct {
	mu      sync.RWMutex
	pending map[string]*ApprovalItem
	history []*ApprovalItem
	maxSize int
	Timeout time.Duration
}

func NewQueue(maxSize int) *Queue {
	return &Queue{
		pending: make(map[string]*ApprovalItem),
		history: make([]*ApprovalItem, 0),
		maxSize: maxSize,
		Timeout: 30 * time.Minute, // default timeout
	}
}

// Submit adds an ActionEnvelope to the approval queue. Returns the approval ID.
func (q *Queue) Submit(env *envelope.ActionEnvelope) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.pending) >= q.maxSize {
		return "", errors.New("approval queue is full")
	}

	now := time.Now().UTC()
	item := &ApprovalItem{
		ID:          env.ID,
		Envelope:    env,
		Status:      StatusPending,
		SubmittedAt: now,
		ExpireAt:    now.Add(q.Timeout),
	}
	q.pending[item.ID] = item
	return item.ID, nil
}

// Pending returns all pending items.
func (q *Queue) Pending() []*ApprovalItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	items := make([]*ApprovalItem, 0, len(q.pending))
	for _, item := range q.pending {
		items = append(items, item)
	}
	return items
}

// Get returns an item by ID (pending or history).
func (q *Queue) Get(id string) (*ApprovalItem, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if item, ok := q.pending[id]; ok {
		return item, nil
	}
	for _, item := range q.history {
		if item.ID == id {
			return item, nil
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
	defer q.mu.Unlock()

	item, ok := q.pending[id]
	if !ok {
		return nil, errors.New("approval item not found or already reviewed: " + id)
	}

	now := time.Now().UTC()
	item.Status = status
	item.ReviewedAt = &now
	item.Reviewer = reviewer
	item.ReviewComment = comment

	delete(q.pending, id)
	q.history = append(q.history, item)

	// Cap history at 1000
	if len(q.history) > 1000 {
		q.history = q.history[len(q.history)-1000:]
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

// CleanupExpired auto-denies items that have exceeded their expiration time.
// Returns the number of items expired.
func (q *Queue) CleanupExpired() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now().UTC()
	expired := 0
	for id, item := range q.pending {
		if now.After(item.ExpireAt) {
			item.Status = StatusExpired
			item.ReviewedAt = &now
			item.Reviewer = "system"
			item.ReviewComment = "auto-denied: approval timeout exceeded"
			delete(q.pending, id)
			q.history = append(q.history, item)
			expired++
		}
	}

	// Cap history at 1000
	if len(q.history) > 1000 {
		q.history = q.history[len(q.history)-1000:]
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
	copy(result, q.history[start:])
	return result
}
