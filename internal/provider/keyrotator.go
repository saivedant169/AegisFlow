package provider

import (
	"sync"
	"time"
)

type keyState int

const (
	keyStateActive keyState = iota
	keyStateRateLimited
	keyStateFailed
)

type managedKey struct {
	value    string
	state    keyState
	cooldown time.Time
}

// KeyRotator selects API keys using round-robin and automatically excludes
// keys that are rate-limited or permanently failed.
type KeyRotator struct {
	mu       sync.Mutex
	keys     []*managedKey
	strategy string
	counter  uint64 // always accessed under mu; plain uint64 avoids drift when active-set length changes
	cooldown time.Duration
}

// NewKeyRotator creates a rotator from the given key values.
// strategy must be "round-robin" (the only supported strategy; defaults to it if empty).
// rateLimitCooldown controls how long a 429-hit key is excluded before being re-admitted.
// TODO: make rateLimitCooldown configurable per-provider via YAML (currently defaults to 60s).
func NewKeyRotator(keys []string, strategy string, rateLimitCooldown time.Duration) *KeyRotator {
	if strategy == "" {
		strategy = "round-robin"
	}
	if rateLimitCooldown == 0 {
		rateLimitCooldown = 60 * time.Second
	}
	managed := make([]*managedKey, 0, len(keys))
	for _, k := range keys {
		if k != "" {
			managed = append(managed, &managedKey{value: k, state: keyStateActive})
		}
	}
	return &KeyRotator{keys: managed, strategy: strategy, cooldown: rateLimitCooldown}
}

// Pick returns the next available API key. Returns ("", false) if no keys are usable.
func (r *KeyRotator) Pick() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	active := r.activeUnlocked()
	if len(active) == 0 {
		return "", false
	}
	idx := r.counter % uint64(len(active))
	r.counter++
	return active[idx].value, true
}

// MarkFailed permanently excludes a key from rotation (call on HTTP 401 Unauthorized).
func (r *KeyRotator) MarkFailed(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, k := range r.keys {
		if k.value == key {
			k.state = keyStateFailed
			return
		}
	}
}

// MarkRateLimited temporarily excludes a key (call on HTTP 429 Too Many Requests).
// The key is re-admitted automatically after the configured cooldown duration.
func (r *KeyRotator) MarkRateLimited(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, k := range r.keys {
		if k.value == key {
			k.state = keyStateRateLimited
			k.cooldown = time.Now().Add(r.cooldown)
			return
		}
	}
}

// Available reports whether at least one key is usable right now.
func (r *KeyRotator) Available() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.activeUnlocked()) > 0
}

// Len returns the total number of configured keys regardless of state.
func (r *KeyRotator) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.keys)
}

// activeUnlocked returns currently usable keys. Caller must hold mu.
// Rate-limited keys whose cooldown has expired are re-admitted automatically.
func (r *KeyRotator) activeUnlocked() []*managedKey {
	now := time.Now()
	var active []*managedKey
	for _, k := range r.keys {
		switch k.state {
		case keyStateActive:
			active = append(active, k)
		case keyStateRateLimited:
			if now.After(k.cooldown) {
				k.state = keyStateActive
				active = append(active, k)
			}
			// keyStateFailed: never re-admitted
		}
	}
	return active
}
