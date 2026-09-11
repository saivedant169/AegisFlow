package evidence

import "fmt"

// AdminAdapter wraps a SessionChain for the admin API.
type AdminAdapter struct {
	tenant string
	scoped bool
	chain  *SessionChain
}

func NewAdminAdapter(chain *SessionChain) *AdminAdapter {
	return &AdminAdapter{chain: chain}
}

func (a *AdminAdapter) ExportSession(sessionID string) (interface{}, error) {
	if a.chain.SessionID() != sessionID || (a.scoped && !chainOwnedBy(a.chain, a.tenant)) {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	return a.chain.Export()
}

func (a *AdminAdapter) VerifySession(sessionID string) (interface{}, error) {
	if a.chain.SessionID() != sessionID || (a.scoped && !chainOwnedBy(a.chain, a.tenant)) {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	records := a.chain.Records()
	return Verify(records), nil
}

func (a *AdminAdapter) ListSessions() (interface{}, error) {
	if a.scoped && !chainOwnedBy(a.chain, a.tenant) {
		return []SessionManifest{}, nil
	}
	manifest := a.chain.Manifest()
	return []SessionManifest{manifest}, nil
}

func (a *AdminAdapter) RenderReport(sessionID string) (string, error) {
	if a.chain.SessionID() != sessionID || (a.scoped && !chainOwnedBy(a.chain, a.tenant)) {
		return "", fmt.Errorf("session %q not found", sessionID)
	}
	return RenderMarkdownReport(a.chain)
}

func (a *AdminAdapter) RenderHTMLReport(sessionID string) (string, error) {
	if a.chain.SessionID() != sessionID || (a.scoped && !chainOwnedBy(a.chain, a.tenant)) {
		return "", fmt.Errorf("session %q not found", sessionID)
	}
	return RenderHTMLReport(a.chain)
}

// ForTenant returns a view limited to evidence owned by one tenant.
func (a *AdminAdapter) ForTenant(tenant string) interface{} {
	return &AdminAdapter{chain: a.chain, tenant: tenant, scoped: true}
}

func chainOwnedBy(chain *SessionChain, tenant string) bool {
	if tenant == "" {
		return false
	}
	records := chain.Records()
	if len(records) == 0 {
		return false
	}
	for _, record := range records {
		if record.Envelope == nil || record.Envelope.Actor.TenantID != tenant {
			return false
		}
	}
	return true
}
