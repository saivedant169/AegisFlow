package credential

import (
	"context"
	"fmt"
	"time"
)

// AdminAdapter exposes credential registry operations for the admin API.
type AdminAdapter struct {
	registry *Registry
}

// NewAdminAdapter wraps a credential Registry for the admin API.
func NewAdminAdapter(registry *Registry) *AdminAdapter {
	return &AdminAdapter{registry: registry}
}

// ActiveCredentials returns all non-expired credentials as a generic interface
// suitable for JSON serialization.
func (a *AdminAdapter) ActiveCredentials() interface{} {
	creds := a.registry.ActiveCredentials()
	if creds == nil {
		return []interface{}{}
	}

	// Redact tokens in the response — only show first 8 chars.
	type redactedCred struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		TokenHint string `json:"token_hint"`
		ExpiresAt string `json:"expires_at"`
		Scope     string `json:"scope"`
		TaskID    string `json:"task_id"`
		IssuedAt  string `json:"issued_at"`
	}

	result := make([]redactedCred, len(creds))
	for i, c := range creds {
		hint := c.Token
		if len(hint) > 8 {
			hint = hint[:8] + "..."
		}
		result[i] = redactedCred{
			ID:        c.ID,
			Type:      c.Type,
			TokenHint: hint,
			ExpiresAt: c.ExpiresAt.Format("2006-01-02T15:04:05Z"),
			Scope:     c.Scope,
			TaskID:    c.TaskID,
			IssuedAt:  c.IssuedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return result
}

// RevokeCredential revokes a credential by ID.
func (a *AdminAdapter) RevokeCredential(id string) error {
	if id == "" {
		return fmt.Errorf("credential ID is required")
	}
	return a.registry.Revoke(nil, id)
}

// IssueCredential issues a credential via the named provider and returns
// the provenance metadata (never the secret). The provenance links the
// credential to the given envelope ID for evidence chain traceability.
func (a *AdminAdapter) IssueCredential(providerName, taskID, target, capability, envelopeID string) (interface{}, error) {
	req := CredentialRequest{
		TaskID:     taskID,
		Target:     target,
		Capability: capability,
		TTL:        15 * time.Minute,
	}
	_, prov, err := a.registry.Issue(context.Background(), providerName, req, envelopeID)
	if err != nil {
		return nil, err
	}
	return prov, nil
}
