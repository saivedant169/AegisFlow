package credential

import (
	"context"
	"fmt"
	"sync"
)

// Registry holds multiple credential brokers and tracks active credentials.
type Registry struct {
	mu      sync.RWMutex
	brokers map[string]Broker
	active  map[string]*Credential // credID -> Credential
}

// NewRegistry creates an empty credential registry.
func NewRegistry() *Registry {
	return &Registry{
		brokers: make(map[string]Broker),
		active:  make(map[string]*Credential),
	}
}

// Register adds a broker to the registry under the given name.
func (r *Registry) Register(name string, broker Broker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.brokers[name] = broker
}

// Issue delegates credential issuance to the named broker and tracks the result.
func (r *Registry) Issue(ctx context.Context, providerName string, req CredentialRequest) (*Credential, error) {
	r.mu.RLock()
	broker, ok := r.brokers[providerName]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("credential broker %q not registered", providerName)
	}

	cred, err := broker.Issue(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("broker %q issue: %w", providerName, err)
	}

	r.mu.Lock()
	r.active[cred.ID] = cred
	r.mu.Unlock()

	return cred, nil
}

// Revoke invalidates a credential by ID, delegating to each registered broker.
func (r *Registry) Revoke(ctx context.Context, credID string) error {
	r.mu.Lock()
	cred, ok := r.active[credID]
	if ok {
		delete(r.active, credID)
	}
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf("credential %q not found", credID)
	}

	// Find the broker by credential type and revoke.
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, broker := range r.brokers {
		if broker.Name() == cred.Type {
			return broker.Revoke(ctx, credID)
		}
	}

	// If no broker matched the type, the credential is already removed from active.
	return nil
}

// ActiveCredentials returns all non-expired credentials currently tracked.
func (r *Registry) ActiveCredentials() []*Credential {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Credential
	for _, cred := range r.active {
		if !cred.Expired() {
			result = append(result, cred)
		}
	}
	return result
}

// CleanupExpired removes all expired credentials from the active set.
func (r *Registry) CleanupExpired() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, cred := range r.active {
		if cred.Expired() {
			delete(r.active, id)
		}
	}
}
