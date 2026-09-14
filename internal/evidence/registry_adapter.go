package evidence

import (
	"encoding/json"
	"fmt"
)

// RegistryAdminAdapter exposes a ChainRegistry through the same admin interface
// the single-chain AdminAdapter uses, but resolves the right per-session chain
// for each call.
type RegistryAdminAdapter struct {
	tenant string
	scoped bool
	reg    *ChainRegistry
}

func NewRegistryAdminAdapter(reg *ChainRegistry) *RegistryAdminAdapter {
	return &RegistryAdminAdapter{reg: reg}
}

func (a *RegistryAdminAdapter) ExportSession(sessionID string) (any, error) {
	chain, err := a.reg.get(sessionID)
	if err != nil {
		return nil, err
	}
	if chain == nil || (a.scoped && !chainOwnedBy(chain, a.tenant)) {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	bundle, err := chain.Export()
	if err != nil {
		return nil, err
	}
	return json.RawMessage(bundle), nil
}

func (a *RegistryAdminAdapter) VerifySession(sessionID string) (any, error) {
	chain, err := a.reg.get(sessionID)
	if err != nil {
		return nil, err
	}
	if chain == nil || (a.scoped && !chainOwnedBy(chain, a.tenant)) {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	// Verify signatures when the registry is keyed, otherwise just the structure.
	if key := a.reg.Key(); len(key) > 0 {
		return VerifySignatures(chain.Records(), key), nil
	}
	return Verify(chain.Records()), nil
}

func (a *RegistryAdminAdapter) ListSessions() (any, error) {
	chains, err := a.reg.all()
	if err != nil {
		return nil, err
	}
	manifests := make([]SessionManifest, 0, len(chains))
	for _, c := range chains {
		if !a.scoped || chainOwnedBy(c, a.tenant) {
			manifests = append(manifests, c.Manifest())
		}
	}
	return manifests, nil
}

func (a *RegistryAdminAdapter) RenderReport(sessionID string) (string, error) {
	chain, err := a.reg.get(sessionID)
	if err != nil {
		return "", err
	}
	if chain == nil || (a.scoped && !chainOwnedBy(chain, a.tenant)) {
		return "", fmt.Errorf("session %q not found", sessionID)
	}
	return RenderMarkdownReport(chain)
}

func (a *RegistryAdminAdapter) RenderHTMLReport(sessionID string) (string, error) {
	chain, err := a.reg.get(sessionID)
	if err != nil {
		return "", err
	}
	if chain == nil || (a.scoped && !chainOwnedBy(chain, a.tenant)) {
		return "", fmt.Errorf("session %q not found", sessionID)
	}
	return RenderHTMLReport(chain)
}

// ForTenant returns a view limited to evidence owned by one tenant.
func (a *RegistryAdminAdapter) ForTenant(tenant string) any {
	return &RegistryAdminAdapter{reg: a.reg, tenant: tenant, scoped: true}
}
