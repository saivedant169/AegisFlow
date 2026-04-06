package supply

import (
	"testing"
)

func TestStrictModeRejectsUnsigned(t *testing.T) {
	key := []byte("test-key-for-verifier-strict!!!!")
	signer := NewSigner(key)
	registry := NewRegistry()
	verifier := NewVerifier(signer, true, registry)

	err := verifier.LoadUnsigned("unsigned-pack", "1.0.0", "policy_pack", []byte("content"))
	if err == nil {
		t.Fatal("expected strict mode to reject unsigned asset")
	}

	// Also test VerifyAndLoad with nil bundle.
	err = verifier.VerifyAndLoad(nil, []byte("content"))
	if err == nil {
		t.Fatal("expected strict mode to reject nil bundle")
	}

	if len(registry.ListAssets()) != 0 {
		t.Error("registry should be empty after rejections")
	}
}

func TestPermissiveModeWarnsUnsigned(t *testing.T) {
	key := []byte("test-key-for-verifier-permiss!!")
	signer := NewSigner(key)
	registry := NewRegistry()
	verifier := NewVerifier(signer, false, registry)

	err := verifier.LoadUnsigned("unsigned-pack", "1.0.0", "policy_pack", []byte("content"))
	if err != nil {
		t.Fatalf("permissive mode should allow unsigned: %v", err)
	}

	assets := registry.ListAssets()
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}
	if assets[0].TrustTier != "unverified" {
		t.Errorf("TrustTier = %q, want %q", assets[0].TrustTier, "unverified")
	}
	if assets[0].Verified {
		t.Error("unsigned asset should not be marked verified")
	}
}

func TestVerifiedAssetLoads(t *testing.T) {
	key := []byte("test-key-for-verifier-verified!")
	signer := NewSigner(key)
	registry := NewRegistry()
	verifier := NewVerifier(signer, true, registry)

	content := []byte("trusted policy content")
	bundle, err := signer.Sign("trusted-pack", "2.0.0", "policy_pack", content)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	err = verifier.VerifyAndLoad(bundle, content)
	if err != nil {
		t.Fatalf("VerifyAndLoad: %v", err)
	}

	assets := registry.ListAssets()
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}
	if assets[0].TrustTier != "verified" {
		t.Errorf("TrustTier = %q, want %q", assets[0].TrustTier, "verified")
	}
	if !assets[0].Verified {
		t.Error("signed asset should be marked verified")
	}
	if assets[0].Name != "trusted-pack" {
		t.Errorf("Name = %q, want %q", assets[0].Name, "trusted-pack")
	}
}
