package admin

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// RequestEntry represents a single request in the live feed.
type RequestEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	RequestID  string    `json:"request_id"`
	TenantID   string    `json:"tenant_id"`
	Model      string    `json:"model"`
	Provider   string    `json:"provider,omitempty"`
	Region     string    `json:"region,omitempty"`
	Status     int       `json:"status"`
	LatencyMs  int64     `json:"latency_ms"`
	Tokens     int       `json:"tokens"`
	Cached     bool      `json:"cached"`
	PolicyHit  string    `json:"policy_hit,omitempty"`
	QualityScore int     `json:"quality_score"`
}

// RequestLog is a thread-safe ring buffer that stores recent requests for the live feed.
type RequestLog struct {
	mu      sync.RWMutex
	entries []RequestEntry
	maxSize int
	pos     int
	count   int
}

func NewRequestLog(maxSize int) *RequestLog {
	if maxSize <= 0 {
		maxSize = 200
	}
	return &RequestLog{
		entries: make([]RequestEntry, maxSize),
		maxSize: maxSize,
	}
}

func (rl *RequestLog) Add(entry RequestEntry) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.entries[rl.pos] = entry
	rl.pos = (rl.pos + 1) % rl.maxSize
	if rl.count < rl.maxSize {
		rl.count++
	}
}

func (rl *RequestLog) Recent(n int) []RequestEntry {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	if n <= 0 || n > rl.count {
		n = rl.count
	}

	result := make([]RequestEntry, 0, n)
	start := (rl.pos - n + rl.maxSize) % rl.maxSize
	for i := 0; i < n; i++ {
		idx := (start + i) % rl.maxSize
		result = append(result, rl.entries[idx])
	}

	// Reverse so newest is first
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// RecentViolations returns the most recent requests that triggered a policy.
func (rl *RequestLog) RecentViolations(n int) []RequestEntry {
	all := rl.Recent(rl.count)
	var violations []RequestEntry
	for _, e := range all {
		if e.PolicyHit != "" {
			violations = append(violations, e)
			if len(violations) >= n {
				break
			}
		}
	}
	return violations
}

// Count returns the number of entries currently stored in the log.
func (rl *RequestLog) Count() int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.count
}

func (rl *RequestLog) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rl.Recent(50))
}
