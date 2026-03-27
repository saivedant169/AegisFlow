package policy

import (
	"os"
	"testing"
	"time"
)

func loadTestWasm(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("failed to load test wasm %s: %v", name, err)
	}
	return data
}

func TestWasmFilterBlocks(t *testing.T) {
	wasm := loadTestWasm(t, "block.wasm")
	f, err := NewWasmFilterFromBytes("test-block", ActionBlock, wasm, time.Second, "block")
	if err != nil {
		t.Fatalf("failed to create wasm filter: %v", err)
	}
	defer f.Close()

	f.SetMetadata(&WasmMetadata{TenantID: "test-tenant", Model: "gpt-4o", Provider: "openai", Phase: "input"})

	v := f.Check("this contains the forbidden word")
	if v == nil {
		t.Fatal("expected violation for content containing 'forbidden'")
	}
	if v.PolicyName != "test-block" {
		t.Errorf("expected policy name 'test-block', got '%s'", v.PolicyName)
	}
	if v.Action != ActionBlock {
		t.Errorf("expected action block, got %s", v.Action)
	}
	if v.Message == "" {
		t.Error("expected non-empty violation message")
	}
}

func TestWasmFilterAllows(t *testing.T) {
	wasm := loadTestWasm(t, "allow.wasm")
	f, err := NewWasmFilterFromBytes("test-allow", ActionBlock, wasm, time.Second, "block")
	if err != nil {
		t.Fatalf("failed to create wasm filter: %v", err)
	}
	defer f.Close()

	f.SetMetadata(&WasmMetadata{TenantID: "t1", Model: "gpt-4o", Provider: "openai", Phase: "input"})

	v := f.Check("this is perfectly fine content")
	if v != nil {
		t.Error("expected no violation for clean content")
	}
}

func TestWasmFilterBlockAllowsCleanContent(t *testing.T) {
	wasm := loadTestWasm(t, "block.wasm")
	f, err := NewWasmFilterFromBytes("test-block", ActionBlock, wasm, time.Second, "block")
	if err != nil {
		t.Fatalf("failed to create wasm filter: %v", err)
	}
	defer f.Close()

	f.SetMetadata(&WasmMetadata{TenantID: "t1", Model: "gpt-4o", Provider: "openai", Phase: "input"})

	v := f.Check("this is normal content without bad words")
	if v != nil {
		t.Error("expected no violation for clean content")
	}
}

func TestWasmFilterBadResultOnErrorBlock(t *testing.T) {
	wasm := loadTestWasm(t, "bad_result.wasm")
	f, err := NewWasmFilterFromBytes("test-bad", ActionBlock, wasm, time.Second, "block")
	if err != nil {
		t.Fatalf("failed to create wasm filter: %v", err)
	}
	defer f.Close()

	f.SetMetadata(&WasmMetadata{TenantID: "t1", Model: "gpt-4o", Provider: "openai", Phase: "input"})

	v := f.Check("anything")
	if v == nil {
		t.Fatal("expected violation from on_error:block when plugin returns bad JSON")
	}
	if v.Action != ActionBlock {
		t.Errorf("expected block action, got %s", v.Action)
	}
}

func TestWasmFilterBadResultOnErrorAllow(t *testing.T) {
	wasm := loadTestWasm(t, "bad_result.wasm")
	f, err := NewWasmFilterFromBytes("test-bad-allow", ActionBlock, wasm, time.Second, "allow")
	if err != nil {
		t.Fatalf("failed to create wasm filter: %v", err)
	}
	defer f.Close()

	f.SetMetadata(&WasmMetadata{TenantID: "t1", Model: "gpt-4o", Provider: "openai", Phase: "input"})

	v := f.Check("anything")
	if v != nil {
		t.Error("expected no violation when on_error is 'allow'")
	}
}

func TestWasmFilterMissingExports(t *testing.T) {
	emptyWasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}
	_, err := NewWasmFilterFromBytes("test-empty", ActionBlock, emptyWasm, time.Second, "block")
	if err == nil {
		t.Fatal("expected error for WASM module missing required exports")
	}
}

func TestWasmFilterNilMetadata(t *testing.T) {
	wasm := loadTestWasm(t, "allow.wasm")
	f, err := NewWasmFilterFromBytes("test-nil-meta", ActionBlock, wasm, time.Second, "block")
	if err != nil {
		t.Fatalf("failed to create wasm filter: %v", err)
	}
	defer f.Close()

	v := f.Check("normal content")
	if v != nil {
		t.Error("expected no violation with nil metadata")
	}
}

func TestWasmFilterConfigActionOverridesPlugin(t *testing.T) {
	wasm := loadTestWasm(t, "block.wasm")
	f, err := NewWasmFilterFromBytes("test-override", ActionWarn, wasm, time.Second, "block")
	if err != nil {
		t.Fatalf("failed to create wasm filter: %v", err)
	}
	defer f.Close()

	f.SetMetadata(&WasmMetadata{TenantID: "t1", Model: "gpt-4o", Provider: "openai", Phase: "input"})

	v := f.Check("this is forbidden content")
	if v == nil {
		t.Fatal("expected violation")
	}
	if v.Action != ActionWarn {
		t.Errorf("expected config action 'warn' to override plugin, got '%s'", v.Action)
	}
}

func TestWasmFilterInEngine(t *testing.T) {
	wasm := loadTestWasm(t, "block.wasm")
	wasmFilter, err := NewWasmFilterFromBytes("wasm-jailbreak", ActionBlock, wasm, time.Second, "block")
	if err != nil {
		t.Fatalf("failed to create wasm filter: %v", err)
	}
	defer wasmFilter.Close()

	wasmFilter.SetMetadata(&WasmMetadata{TenantID: "t1", Model: "gpt-4o", Provider: "openai", Phase: "input"})

	kwFilter := NewKeywordFilter("kw-test", ActionBlock, []string{"blocked_word"})
	engine := NewEngine([]Filter{kwFilter, wasmFilter}, nil)

	v, _ := engine.CheckInput("this has blocked_word in it")
	if v == nil || v.PolicyName != "kw-test" {
		t.Error("expected keyword filter to catch 'blocked_word'")
	}

	v, _ = engine.CheckInput("this has forbidden in it")
	if v == nil || v.PolicyName != "wasm-jailbreak" {
		t.Errorf("expected wasm filter to catch 'forbidden', got %v", v)
	}

	v, _ = engine.CheckInput("this is perfectly clean")
	if v != nil {
		t.Error("expected no violation for clean input")
	}
}
