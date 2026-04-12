package evidence

import "fmt"

// AdminAdapter wraps a SessionChain for the admin API.
type AdminAdapter struct {
	chain *SessionChain
}

func NewAdminAdapter(chain *SessionChain) *AdminAdapter {
	return &AdminAdapter{chain: chain}
}

func (a *AdminAdapter) ExportSession(sessionID string) (interface{}, error) {
	if a.chain.SessionID() != sessionID {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	return a.chain.Export()
}

func (a *AdminAdapter) VerifySession(sessionID string) (interface{}, error) {
	if a.chain.SessionID() != sessionID {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	records := a.chain.Records()
	return Verify(records), nil
}

func (a *AdminAdapter) ListSessions() interface{} {
	manifest := a.chain.Manifest()
	return []SessionManifest{manifest}
}

func (a *AdminAdapter) RenderReport(sessionID string) (string, error) {
	if a.chain.SessionID() != sessionID {
		return "", fmt.Errorf("session %q not found", sessionID)
	}
	return RenderMarkdownReport(a.chain)
}

func (a *AdminAdapter) RenderHTMLReport(sessionID string) (string, error) {
	if a.chain.SessionID() != sessionID {
		return "", fmt.Errorf("session %q not found", sessionID)
	}
	return RenderHTMLReport(a.chain)
}
