package supply

import (
	"fmt"
	"log"
)

// Verifier checks extension bundles at load time and enforces trust policy.
type Verifier struct {
	signer     *Signer
	strictMode bool
	registry   *Registry
}

// NewVerifier creates a Verifier. In strict mode unsigned assets are rejected;
// in permissive mode they are loaded with a warning and marked "unverified".
func NewVerifier(signer *Signer, strictMode bool, registry *Registry) *Verifier {
	return &Verifier{
		signer:     signer,
		strictMode: strictMode,
		registry:   registry,
	}
}

// VerifyAndLoad checks the bundle signature (if present) and registers the
// asset in the registry. Returns an error if verification fails or if strict
// mode rejects an unsigned asset.
func (v *Verifier) VerifyAndLoad(bundle *SignedBundle, content []byte) error {
	if bundle == nil {
		// Unsigned asset.
		if v.strictMode {
			return fmt.Errorf("supply: strict mode rejects unsigned assets")
		}
		log.Println("supply: WARNING: loading unsigned asset (permissive mode)")
		return nil
	}

	err := v.signer.Verify(bundle, content)
	if err != nil {
		return fmt.Errorf("supply: verification failed: %w", err)
	}

	tier := bundle.TrustTier
	if tier == "" {
		tier = "community"
	}

	v.registry.Register(LoadedAsset{
		Name:        bundle.Name,
		Version:     bundle.Version,
		Type:        bundle.Type,
		TrustTier:   tier,
		ContentHash: bundle.ContentHash,
		Verified:    true,
	})

	return nil
}

// LoadUnsigned registers an unsigned asset in permissive mode.
// Returns an error in strict mode.
func (v *Verifier) LoadUnsigned(name, version, assetType string, content []byte) error {
	if v.strictMode {
		return fmt.Errorf("supply: strict mode rejects unsigned asset %q", name)
	}
	log.Printf("supply: WARNING: loading unsigned asset %q (permissive mode)", name)

	v.registry.Register(LoadedAsset{
		Name:        name,
		Version:     version,
		Type:        assetType,
		TrustTier:   "unverified",
		ContentHash: sha256Hex(content),
		Verified:    false,
	})

	return nil
}

// IsStrict reports whether the verifier is in strict mode.
func (v *Verifier) IsStrict() bool {
	return v.strictMode
}
