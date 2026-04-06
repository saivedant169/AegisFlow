package mcpgw

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// SSEEvent represents a Server-Sent Event.
type SSEEvent struct {
	Event string // "endpoint", "message"
	Data  string // JSON-RPC response or endpoint URL
	ID    string // optional event ID
}

// SSESession tracks a single SSE client connection.
type SSESession struct {
	ID      string
	Events  chan SSEEvent
	Done    chan struct{}
	Created time.Time
}

// SSEManager manages active SSE sessions.
type SSEManager struct {
	mu       sync.RWMutex
	sessions map[string]*SSESession
}

// NewSSEManager creates a new SSE session manager.
func NewSSEManager() *SSEManager {
	return &SSEManager{
		sessions: make(map[string]*SSESession),
	}
}

// CreateSession creates a new SSE session with a random ID.
func (m *SSEManager) CreateSession() *SSESession {
	id := generateSessionID()
	s := &SSESession{
		ID:      id,
		Events:  make(chan SSEEvent, 64),
		Done:    make(chan struct{}),
		Created: time.Now(),
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	return s
}

// GetSession retrieves a session by ID, or nil if not found.
func (m *SSEManager) GetSession(id string) *SSESession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// RemoveSession removes and closes a session.
func (m *SSEManager) RemoveSession(id string) {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if ok {
		select {
		case <-s.Done:
		default:
			close(s.Done)
		}
	}
}

// CleanupStale removes sessions older than maxAge.
func (m *SSEManager) CleanupStale(maxAge time.Duration) {
	now := time.Now()
	m.mu.Lock()
	var stale []string
	for id, s := range m.sessions {
		if now.Sub(s.Created) > maxAge {
			stale = append(stale, id)
		}
	}
	for _, id := range stale {
		s := m.sessions[id]
		delete(m.sessions, id)
		select {
		case <-s.Done:
		default:
			close(s.Done)
		}
	}
	m.mu.Unlock()
}

// Format returns the SSE wire format for the event.
func (e SSEEvent) Format() string {
	var out string
	if e.ID != "" {
		out += fmt.Sprintf("id: %s\n", e.ID)
	}
	if e.Event != "" {
		out += fmt.Sprintf("event: %s\n", e.Event)
	}
	out += fmt.Sprintf("data: %s\n\n", e.Data)
	return out
}

func generateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
