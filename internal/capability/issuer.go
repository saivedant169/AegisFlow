package capability

import (
	"crypto/hmac"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Issuer creates, signs, verifies, and revokes capability tickets.
type Issuer struct {
	signingKey []byte
	store      *Store
}

// NewIssuer creates a new Issuer with the given HMAC signing key.
func NewIssuer(signingKey []byte) *Issuer {
	return &Issuer{
		signingKey: signingKey,
		store:      NewStore(),
	}
}

// Issue creates and signs a new capability ticket from the given request.
func (iss *Issuer) Issue(req TicketRequest) (*Ticket, error) {
	nonce, err := generateNonce()
	if err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	now := time.Now().UTC()
	ttl := req.TTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	t := &Ticket{
		ID:           uuid.New().String(),
		Subject:      req.Subject,
		TaskID:       req.TaskID,
		SessionID:    req.SessionID,
		EnvelopeID:   req.EnvelopeID,
		Resource:     req.Resource,
		Verb:         req.Verb,
		Protocol:     req.Protocol,
		Tool:         req.Tool,
		PolicyHash:   req.PolicyHash,
		ApprovalHash: req.ApprovalHash,
		IssuedAt:     now,
		ExpiresAt:    now.Add(ttl),
		Nonce:        nonce,
		EvidenceRef:  req.EvidenceRef,
	}

	sig, err := computeSignature(t, iss.signingKey)
	if err != nil {
		return nil, fmt.Errorf("signing ticket: %w", err)
	}
	t.Signature = sig

	iss.store.Add(t)
	return t, nil
}

// Verify checks that a ticket's signature is valid, it has not expired,
// and it has not been revoked.
func (iss *Issuer) Verify(ticket *Ticket) error {
	// Check revocation first.
	if iss.store.IsRevoked(ticket.ID) {
		return fmt.Errorf("ticket %s has been revoked", ticket.ID)
	}

	// Check expiry.
	if ticket.Expired() {
		return fmt.Errorf("ticket %s has expired", ticket.ID)
	}

	// Verify signature.
	expected, err := computeSignature(ticket, iss.signingKey)
	if err != nil {
		return fmt.Errorf("computing expected signature: %w", err)
	}

	expectedBytes, _ := hex.DecodeString(expected)
	actualBytes, err := hex.DecodeString(ticket.Signature)
	if err != nil {
		return fmt.Errorf("decoding ticket signature: %w", err)
	}

	if !hmac.Equal(expectedBytes, actualBytes) {
		return fmt.Errorf("ticket %s signature is invalid", ticket.ID)
	}

	return nil
}

// Revoke adds the ticket ID to the revocation list.
func (iss *Issuer) Revoke(ticketID string) {
	iss.store.Revoke(ticketID)
}

// IsRevoked returns true if the ticket has been revoked.
func (iss *Issuer) IsRevoked(ticketID string) bool {
	return iss.store.IsRevoked(ticketID)
}

// Store returns the issuer's ticket store.
func (iss *Issuer) Store() *Store {
	return iss.store
}

// generateNonce creates a cryptographically random 16-byte hex string.
func generateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
