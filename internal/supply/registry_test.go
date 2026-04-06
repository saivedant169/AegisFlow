package supply

import (
	"testing"
)

func TestRegistryListAssets(t *testing.T) {
	reg := NewRegistry()

	// Empty registry.
	if assets := reg.ListAssets(); len(assets) != 0 {
		t.Fatalf("expected 0 assets, got %d", len(assets))
	}

	reg.Register(LoadedAsset{
		Name:        "pack-a",
		Version:     "1.0.0",
		Type:        "policy_pack",
		TrustTier:   "verified",
		ContentHash: "abc123",
		Verified:    true,
	})
	reg.Register(LoadedAsset{
		Name:        "plugin-b",
		Version:     "2.0.0",
		Type:        "wasm_plugin",
		TrustTier:   "community",
		ContentHash: "def456",
		Verified:    true,
	})

	assets := reg.ListAssets()
	if len(assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(assets))
	}
	if assets[0].Name != "pack-a" {
		t.Errorf("assets[0].Name = %q, want %q", assets[0].Name, "pack-a")
	}
	if assets[1].Name != "plugin-b" {
		t.Errorf("assets[1].Name = %q, want %q", assets[1].Name, "plugin-b")
	}

	// Verify LoadedAt is set.
	for i, a := range assets {
		if a.LoadedAt.IsZero() {
			t.Errorf("assets[%d].LoadedAt is zero", i)
		}
	}

	// Verify returned slice is a copy.
	assets[0].Name = "mutated"
	fresh := reg.ListAssets()
	if fresh[0].Name == "mutated" {
		t.Error("ListAssets should return a copy, not a reference to internal state")
	}
}

func TestRegistryTrustTierTracking(t *testing.T) {
	reg := NewRegistry()

	reg.Register(LoadedAsset{Name: "a", TrustTier: "verified"})
	reg.Register(LoadedAsset{Name: "b", TrustTier: "verified"})
	reg.Register(LoadedAsset{Name: "c", TrustTier: "community"})
	reg.Register(LoadedAsset{Name: "d", TrustTier: "unverified"})

	counts := reg.CountByTrustTier()
	if counts["verified"] != 2 {
		t.Errorf("verified count = %d, want 2", counts["verified"])
	}
	if counts["community"] != 1 {
		t.Errorf("community count = %d, want 1", counts["community"])
	}
	if counts["unverified"] != 1 {
		t.Errorf("unverified count = %d, want 1", counts["unverified"])
	}
}
