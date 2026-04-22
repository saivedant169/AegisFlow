package supply

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// SignedBundle contains the metadata and cryptographic proof for a signed
// extension (policy pack, WASM plugin, or connector).
type SignedBundle struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Type        string    `json:"type"`         // "policy_pack", "wasm_plugin", "connector"
	ContentHash string    `json:"content_hash"` // SHA-256 of content
	Signature   string    `json:"signature"`    // HMAC-SHA256 of metadata+content_hash
	SignedAt    time.Time `json:"signed_at"`
	SignedBy    string    `json:"signed_by"`
	TrustTier   string    `json:"trust_tier"` // "verified", "community", "unverified"
}

// Signer creates and verifies HMAC-SHA256 signatures for extension bundles.
type Signer struct {
	key []byte
}

// NewSigner returns a Signer that uses the given HMAC key.
func NewSigner(key []byte) *Signer {
	return &Signer{key: key}
}

// Sign produces a SignedBundle for the given extension content.
func (s *Signer) Sign(name, version, bundleType string, content []byte) (*SignedBundle, error) {
	if name == "" {
		return nil, fmt.Errorf("supply: name is required")
	}
	if version == "" {
		return nil, fmt.Errorf("supply: version is required")
	}
	switch bundleType {
	case "policy_pack", "wasm_plugin", "connector":
		// valid
	default:
		return nil, fmt.Errorf("supply: invalid bundle type %q", bundleType)
	}

	contentHash := sha256Hex(content)
	sig := s.computeSignature(name, version, bundleType, contentHash)

	return &SignedBundle{
		Name:        name,
		Version:     version,
		Type:        bundleType,
		ContentHash: contentHash,
		Signature:   sig,
		SignedAt:    time.Now().UTC(),
		SignedBy:    "aegisflow-signer",
		TrustTier:   "verified",
	}, nil
}

// Verify checks that a SignedBundle matches the given content and has a valid
// HMAC signature.
func (s *Signer) Verify(bundle *SignedBundle, content []byte) error {
	if bundle == nil {
		return fmt.Errorf("supply: bundle is nil")
	}

	// Verify content hash.
	actualHash := sha256Hex(content)
	if actualHash != bundle.ContentHash {
		return fmt.Errorf("supply: content hash mismatch: expected %s, got %s", bundle.ContentHash, actualHash)
	}

	// Verify HMAC signature.
	expectedSig := s.computeSignature(bundle.Name, bundle.Version, bundle.Type, bundle.ContentHash)
	if !hmac.Equal([]byte(expectedSig), []byte(bundle.Signature)) {
		return fmt.Errorf("supply: signature verification failed")
	}

	return nil
}

// computeSignature returns the hex-encoded HMAC-SHA256 of the canonical
// metadata string: "name|version|type|content_hash".
func (s *Signer) computeSignature(name, version, bundleType, contentHash string) string {
	mac := hmac.New(sha256.New, s.key)
	payload := name + "|" + version + "|" + bundleType + "|" + contentHash
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
