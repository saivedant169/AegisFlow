package credential

import "time"

// CredentialProvenance captures non-secret metadata about an issued credential,
// suitable for inclusion in evidence chains and audit records.
type CredentialProvenance struct {
	CredentialID string    `json:"credential_id"`
	BrokerName   string    `json:"broker_name"`
	Type         string    `json:"type"`
	Scope        string    `json:"scope"`
	IssuedAt     time.Time `json:"issued_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	TaskID       string    `json:"task_id"`
	EnvelopeID   string    `json:"envelope_id"`
}

// ToProvenance extracts non-secret metadata from a Credential and links it to
// the given envelope ID. The returned value is safe for logging, JSON
// serialization, and embedding in evidence records.
func ToProvenance(cred *Credential, brokerName, envelopeID string) *CredentialProvenance {
	if cred == nil {
		return nil
	}
	return &CredentialProvenance{
		CredentialID: cred.ID,
		BrokerName:   brokerName,
		Type:         cred.Type,
		Scope:        cred.Scope,
		IssuedAt:     cred.IssuedAt,
		ExpiresAt:    cred.ExpiresAt,
		TaskID:       cred.TaskID,
		EnvelopeID:   envelopeID,
	}
}
