package credential

import (
	"context"
	"time"
)

// Credential represents a short-lived, task-scoped credential issued by a broker.
type Credential struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`       // "github_app", "aws_sts", "vault", "static"
	Token     string    `json:"token"`      // the actual credential value
	ExpiresAt time.Time `json:"expires_at"`
	Scope     string    `json:"scope"`      // what this credential can access
	TaskID    string    `json:"task_id"`    // which task/session this was issued for
	IssuedAt  time.Time `json:"issued_at"`
}

// Expired returns true if the credential has passed its expiration time.
func (c *Credential) Expired() bool {
	return time.Now().After(c.ExpiresAt)
}

// Broker issues and revokes short-lived credentials for a specific provider type.
type Broker interface {
	// Issue creates a new short-lived credential for the given request.
	Issue(ctx context.Context, req CredentialRequest) (*Credential, error)
	// Revoke invalidates a previously issued credential.
	Revoke(ctx context.Context, credID string) error
	// Name returns the provider name this broker handles.
	Name() string
}

// CredentialRequest describes what credential is needed.
type CredentialRequest struct {
	TaskID     string        `json:"task_id"`
	SessionID  string        `json:"session_id"`
	TenantID   string        `json:"tenant_id"`
	Tool       string        `json:"tool"`
	Target     string        `json:"target"`
	Capability string        `json:"capability"`
	TTL        time.Duration `json:"ttl"`
}
