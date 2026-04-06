package capability

import "fmt"

// AdminAdapter exposes capability ticket operations for the admin API.
type AdminAdapter struct {
	issuer *Issuer
}

// NewAdminAdapter wraps an Issuer for the admin API.
func NewAdminAdapter(issuer *Issuer) *AdminAdapter {
	return &AdminAdapter{issuer: issuer}
}

// ActiveTickets returns all active tickets suitable for JSON serialization.
func (a *AdminAdapter) ActiveTickets() interface{} {
	tickets := a.issuer.Store().ActiveTickets()
	if tickets == nil {
		return []interface{}{}
	}
	return tickets
}

// RevokeTicket revokes a ticket by ID.
func (a *AdminAdapter) RevokeTicket(id string) error {
	if id == "" {
		return fmt.Errorf("ticket ID is required")
	}
	a.issuer.Revoke(id)
	return nil
}

// VerifyTicket verifies a ticket by ID and returns the result.
func (a *AdminAdapter) VerifyTicket(id string) (interface{}, error) {
	if id == "" {
		return nil, fmt.Errorf("ticket ID is required")
	}

	ticket := a.issuer.Store().Get(id)
	if ticket == nil {
		return nil, fmt.Errorf("ticket %q not found", id)
	}

	err := a.issuer.Verify(ticket)
	result := map[string]interface{}{
		"ticket_id": id,
		"valid":     err == nil,
	}
	if err != nil {
		result["error"] = err.Error()
	}
	return result, nil
}
