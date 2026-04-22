package capability

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Ticket is a signed, one-purpose execution capability.
type Ticket struct {
	ID           string    `json:"id"`
	Subject      string    `json:"subject"` // agent/user ID
	TaskID       string    `json:"task_id"`
	SessionID    string    `json:"session_id"`
	EnvelopeID   string    `json:"envelope_id"` // the action this authorizes
	Resource     string    `json:"resource"`    // exact resource
	Verb         string    `json:"verb"`        // exact verb
	Protocol     string    `json:"protocol"`
	Tool         string    `json:"tool"`
	PolicyHash   string    `json:"policy_hash"`             // hash of the policy that authorized this
	ApprovalHash string    `json:"approval_hash,omitempty"` // hash of approval if review path
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	Nonce        string    `json:"nonce"`        // replay protection
	EvidenceRef  string    `json:"evidence_ref"` // evidence chain pointer
	Signature    string    `json:"signature"`    // HMAC-SHA256 signature
}

// TicketRequest describes the parameters needed to issue a capability ticket.
type TicketRequest struct {
	Subject      string        `json:"subject"`
	TaskID       string        `json:"task_id"`
	SessionID    string        `json:"session_id"`
	EnvelopeID   string        `json:"envelope_id"`
	Resource     string        `json:"resource"`
	Verb         string        `json:"verb"`
	Protocol     string        `json:"protocol"`
	Tool         string        `json:"tool"`
	PolicyHash   string        `json:"policy_hash"`
	ApprovalHash string        `json:"approval_hash,omitempty"`
	EvidenceRef  string        `json:"evidence_ref"`
	TTL          time.Duration `json:"ttl"`
}

// canonicalJSON returns the deterministic JSON of all ticket fields except Signature.
func (t *Ticket) canonicalJSON() ([]byte, error) {
	canonical := struct {
		ID           string    `json:"id"`
		Subject      string    `json:"subject"`
		TaskID       string    `json:"task_id"`
		SessionID    string    `json:"session_id"`
		EnvelopeID   string    `json:"envelope_id"`
		Resource     string    `json:"resource"`
		Verb         string    `json:"verb"`
		Protocol     string    `json:"protocol"`
		Tool         string    `json:"tool"`
		PolicyHash   string    `json:"policy_hash"`
		ApprovalHash string    `json:"approval_hash,omitempty"`
		IssuedAt     time.Time `json:"issued_at"`
		ExpiresAt    time.Time `json:"expires_at"`
		Nonce        string    `json:"nonce"`
		EvidenceRef  string    `json:"evidence_ref"`
	}{
		ID:           t.ID,
		Subject:      t.Subject,
		TaskID:       t.TaskID,
		SessionID:    t.SessionID,
		EnvelopeID:   t.EnvelopeID,
		Resource:     t.Resource,
		Verb:         t.Verb,
		Protocol:     t.Protocol,
		Tool:         t.Tool,
		PolicyHash:   t.PolicyHash,
		ApprovalHash: t.ApprovalHash,
		IssuedAt:     t.IssuedAt,
		ExpiresAt:    t.ExpiresAt,
		Nonce:        t.Nonce,
		EvidenceRef:  t.EvidenceRef,
	}
	return json.Marshal(canonical)
}

// computeSignature computes the HMAC-SHA256 signature over the canonical JSON.
func computeSignature(t *Ticket, key []byte) (string, error) {
	data, err := t.canonicalJSON()
	if err != nil {
		return "", fmt.Errorf("canonical JSON: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// Expired returns true if the ticket has passed its expiration time.
func (t *Ticket) Expired() bool {
	return time.Now().After(t.ExpiresAt)
}
