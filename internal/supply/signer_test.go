package supply

import (
	"testing"
)

func TestSignAndVerify(t *testing.T) {
	key := []byte("test-signing-key-32-bytes-long!!")
	signer := NewSigner(key)

	content := []byte("policy pack content here")
	bundle, err := signer.Sign("my-policy", "1.0.0", "policy_pack", content)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if bundle.Name != "my-policy" {
		t.Errorf("Name = %q, want %q", bundle.Name, "my-policy")
	}
	if bundle.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", bundle.Version, "1.0.0")
	}
	if bundle.TrustTier != "verified" {
		t.Errorf("TrustTier = %q, want %q", bundle.TrustTier, "verified")
	}
	if bundle.Signature == "" {
		t.Error("Signature is empty")
	}
	if bundle.ContentHash == "" {
		t.Error("ContentHash is empty")
	}

	if err := signer.Verify(bundle, content); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyTamperedContent(t *testing.T) {
	key := []byte("test-signing-key-32-bytes-long!!")
	signer := NewSigner(key)

	content := []byte("original content")
	bundle, err := signer.Sign("pack", "1.0.0", "policy_pack", content)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	tampered := []byte("tampered content")
	err = signer.Verify(bundle, tampered)
	if err == nil {
		t.Fatal("expected error for tampered content, got nil")
	}
}

func TestVerifyTamperedSignature(t *testing.T) {
	key := []byte("test-signing-key-32-bytes-long!!")
	signer := NewSigner(key)

	content := []byte("some content")
	bundle, err := signer.Sign("pack", "1.0.0", "wasm_plugin", content)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	bundle.Signature = "deadbeef" + bundle.Signature[8:]
	err = signer.Verify(bundle, content)
	if err == nil {
		t.Fatal("expected error for tampered signature, got nil")
	}
}

func TestTrustTiers(t *testing.T) {
	key := []byte("test-signing-key-32-bytes-long!!")
	signer := NewSigner(key)

	tests := []struct {
		bundleType string
	}{
		{"policy_pack"},
		{"wasm_plugin"},
		{"connector"},
	}

	for _, tt := range tests {
		t.Run(tt.bundleType, func(t *testing.T) {
			content := []byte("content for " + tt.bundleType)
			bundle, err := signer.Sign("ext-"+tt.bundleType, "1.0.0", tt.bundleType, content)
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			if bundle.TrustTier != "verified" {
				t.Errorf("TrustTier = %q, want %q", bundle.TrustTier, "verified")
			}
			if bundle.Type != tt.bundleType {
				t.Errorf("Type = %q, want %q", bundle.Type, tt.bundleType)
			}
			if err := signer.Verify(bundle, content); err != nil {
				t.Errorf("Verify: %v", err)
			}
		})
	}

	// Invalid type should fail.
	_, err := signer.Sign("bad", "1.0.0", "invalid_type", []byte("x"))
	if err == nil {
		t.Error("expected error for invalid bundle type")
	}
}
