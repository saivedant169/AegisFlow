package credential

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// StaticBroker is a fallback credential broker that wraps a pre-configured token.
// It always returns the same token value, wrapped with a TTL. This is intended
// as a degraded-mode fallback when dynamic credential providers are unavailable.
type StaticBroker struct {
	name  string
	token string
	ttl   time.Duration

	mu      sync.Mutex
	revoked map[string]bool
}

// NewStaticBroker creates a new static credential broker.
func NewStaticBroker(name, token string, ttl time.Duration) *StaticBroker {
	return &StaticBroker{
		name:    name,
		token:   token,
		ttl:     ttl,
		revoked: make(map[string]bool),
	}
}

// Name returns the broker name.
func (b *StaticBroker) Name() string {
	return b.name
}

// Issue returns a credential wrapping the static token with the configured TTL.
func (b *StaticBroker) Issue(_ context.Context, req CredentialRequest) (*Credential, error) {
	log.Printf("[credential] WARNING: issuing static credential for task %s — this is degraded mode, use a dynamic provider in production", req.TaskID)

	ttl := b.ttl
	if req.TTL > 0 {
		ttl = req.TTL
	}

	now := time.Now().UTC()
	cred := &Credential{
		ID:        uuid.New().String(),
		Type:      "static",
		Token:     b.token,
		ExpiresAt: now.Add(ttl),
		Scope:     req.Target,
		TaskID:    req.TaskID,
		IssuedAt:  now,
	}

	return cred, nil
}

// Revoke marks a credential as revoked. Since static tokens can't truly be
// invalidated, this only records the revocation locally.
func (b *StaticBroker) Revoke(_ context.Context, credID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.revoked[credID] = true
	log.Printf("[credential] static credential %s revoked (note: underlying token unchanged)", credID)
	return nil
}
