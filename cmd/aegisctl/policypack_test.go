package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPolicyPack_Valid(t *testing.T) {
	content := `
name: test-pack
description: "A test policy pack"
default_decision: block
rules:
  - protocol: "*"
    tool: "get_*"
    decision: allow
  - protocol: "*"
    tool: "delete_*"
    decision: block
`
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	pack, err := LoadPolicyPack(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pack.Name != "test-pack" {
		t.Fatalf("expected name test-pack, got %s", pack.Name)
	}
	if pack.DefaultDecision != "block" {
		t.Fatalf("expected default_decision block, got %s", pack.DefaultDecision)
	}
	if len(pack.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(pack.Rules))
	}
	if pack.Rules[0].Decision != "allow" {
		t.Fatalf("expected first rule allow, got %s", pack.Rules[0].Decision)
	}
}

func TestLoadPolicyPack_MissingName(t *testing.T) {
	content := `
default_decision: block
rules:
  - protocol: "*"
    tool: "*"
    decision: block
`
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	os.WriteFile(path, []byte(content), 0644)

	_, err := LoadPolicyPack(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestLoadPolicyPack_InvalidDecision(t *testing.T) {
	content := `
name: bad-decision
default_decision: block
rules:
  - protocol: "*"
    tool: "*"
    decision: yolo
`
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	os.WriteFile(path, []byte(content), 0644)

	_, err := LoadPolicyPack(path)
	if err == nil {
		t.Fatal("expected error for invalid decision")
	}
}

func TestLoadPolicyPack_NoRules(t *testing.T) {
	content := `
name: empty-rules
default_decision: allow
rules: []
`
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	os.WriteFile(path, []byte(content), 0644)

	_, err := LoadPolicyPack(path)
	if err == nil {
		t.Fatal("expected error for empty rules")
	}
}

func TestListPolicyPacks(t *testing.T) {
	dir := t.TempDir()

	pack1 := `
name: pack-one
description: "First pack"
default_decision: block
rules:
  - protocol: "*"
    tool: "*"
    decision: block
`
	pack2 := `
name: pack-two
description: "Second pack"
default_decision: allow
rules:
  - protocol: "*"
    tool: "get_*"
    decision: allow
`
	os.WriteFile(filepath.Join(dir, "pack1.yaml"), []byte(pack1), 0644)
	os.WriteFile(filepath.Join(dir, "pack2.yaml"), []byte(pack2), 0644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a yaml"), 0644)

	packs, err := ListPolicyPacks(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(packs) != 2 {
		t.Fatalf("expected 2 packs, got %d", len(packs))
	}
}

func TestLoadRealPolicyPacks(t *testing.T) {
	// Test loading the actual policy packs shipped with AegisFlow
	dir := filepath.Join("../..", "configs", "policy-packs")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skip("policy-packs directory not found, skipping")
	}

	packs, err := ListPolicyPacks(dir)
	if err != nil {
		t.Fatalf("failed to list real policy packs: %v", err)
	}
	if len(packs) < 3 {
		t.Fatalf("expected at least 3 policy packs, got %d", len(packs))
	}

	names := make(map[string]bool)
	for _, p := range packs {
		names[p.Name] = true
	}

	expected := []string{"coding-agent-strict", "coding-agent-permissive", "coding-agent-readonly"}
	for _, name := range expected {
		if !names[name] {
			t.Fatalf("expected policy pack %q not found", name)
		}
	}
}
