package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// WasmMetadata holds request context passed to WASM plugins.
type WasmMetadata struct {
	TenantID string `json:"tenant_id"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
	Phase    string `json:"phase"`
}

// WasmFilter runs a WASM module as a policy filter.
type WasmFilter struct {
	name    string
	action  Action
	onError string
	timeout time.Duration

	runtime wazero.Runtime
	module  api.Module
	mu      sync.Mutex

	metadata *WasmMetadata
}

type wasmResult struct {
	Action  string `json:"action"`
	Message string `json:"message"`
}

func NewWasmFilter(name string, action Action, wasmPath string, timeout time.Duration, onError string) (*WasmFilter, error) {
	if timeout == 0 {
		timeout = 100 * time.Millisecond
	}
	if onError == "" {
		onError = "block"
	}

	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("reading wasm file %s: %w", wasmPath, err)
	}

	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("compiling wasm module %s: %w", wasmPath, err)
	}

	cfg := wasmModuleConfig(compiled)
	module, err := rt.InstantiateModule(ctx, compiled, cfg)
	if err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("instantiating wasm module %s: %w", wasmPath, err)
	}

	for _, fname := range []string{"check", "alloc", "get_result_ptr", "get_result_len"} {
		if module.ExportedFunction(fname) == nil {
			module.Close(ctx)
			rt.Close(ctx)
			return nil, fmt.Errorf("wasm module %s missing required export: %s", wasmPath, fname)
		}
	}

	return &WasmFilter{
		name:    name,
		action:  action,
		onError: onError,
		timeout: timeout,
		runtime: rt,
		module:  module,
	}, nil
}

func NewWasmFilterFromBytes(name string, action Action, wasmBytes []byte, timeout time.Duration, onError string) (*WasmFilter, error) {
	if timeout == 0 {
		timeout = 100 * time.Millisecond
	}
	if onError == "" {
		onError = "block"
	}

	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("compiling wasm module: %w", err)
	}

	cfg := wasmModuleConfig(compiled)
	module, err := rt.InstantiateModule(ctx, compiled, cfg)
	if err != nil {
		rt.Close(ctx)
		return nil, fmt.Errorf("instantiating wasm module: %w", err)
	}

	for _, fname := range []string{"check", "alloc", "get_result_ptr", "get_result_len"} {
		if module.ExportedFunction(fname) == nil {
			module.Close(ctx)
			rt.Close(ctx)
			return nil, fmt.Errorf("wasm module missing required export: %s", fname)
		}
	}

	return &WasmFilter{
		name:    name,
		action:  action,
		onError: onError,
		timeout: timeout,
		runtime: rt,
		module:  module,
	}, nil
}

// wasmModuleConfig returns a module config that calls _initialize for reactor-style
// modules or _start for command-style modules.
func wasmModuleConfig(compiled wazero.CompiledModule) wazero.ModuleConfig {
	cfg := wazero.NewModuleConfig()
	exports := compiled.ExportedFunctions()
	if _, ok := exports["_initialize"]; ok {
		return cfg.WithStartFunctions("_initialize")
	}
	return cfg
}

func (f *WasmFilter) Name() string   { return f.name }
func (f *WasmFilter) Action() Action { return f.action }

func (f *WasmFilter) SetMetadata(meta *WasmMetadata) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.metadata = meta
}

func (f *WasmFilter) Check(content string) *Violation {
	f.mu.Lock()
	meta := f.metadata
	f.mu.Unlock()

	if meta == nil {
		meta = &WasmMetadata{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()

	violation, err := f.callCheck(ctx, content, meta)
	if err != nil {
		log.Printf("wasm plugin %s error: %v", f.name, err)
		if f.onError == "block" {
			return &Violation{
				PolicyName: f.name,
				Action:     f.action,
				Message:    fmt.Sprintf("wasm plugin error: %s: %v", f.name, err),
			}
		}
		return nil
	}

	return violation
}

func (f *WasmFilter) callCheck(ctx context.Context, content string, meta *WasmMetadata) (*Violation, error) {
	allocFn := f.module.ExportedFunction("alloc")
	checkFn := f.module.ExportedFunction("check")
	getResultPtrFn := f.module.ExportedFunction("get_result_ptr")
	getResultLenFn := f.module.ExportedFunction("get_result_len")

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshaling metadata: %w", err)
	}

	contentBytes := []byte(content)

	contentResults, err := allocFn.Call(ctx, uint64(len(contentBytes)))
	if err != nil {
		return nil, fmt.Errorf("alloc for content: %w", err)
	}
	contentPtr := uint32(contentResults[0])

	if !f.module.Memory().Write(contentPtr, contentBytes) {
		return nil, fmt.Errorf("writing content to wasm memory")
	}

	metaResults, err := allocFn.Call(ctx, uint64(len(metaJSON)))
	if err != nil {
		return nil, fmt.Errorf("alloc for metadata: %w", err)
	}
	metaPtr := uint32(metaResults[0])

	if !f.module.Memory().Write(metaPtr, metaJSON) {
		return nil, fmt.Errorf("writing metadata to wasm memory")
	}

	checkResults, err := checkFn.Call(ctx,
		uint64(contentPtr), uint64(len(contentBytes)),
		uint64(metaPtr), uint64(len(metaJSON)),
	)
	if err != nil {
		return nil, fmt.Errorf("calling check: %w", err)
	}

	result := int32(checkResults[0])
	if result == 0 {
		return nil, nil
	}

	ptrResults, err := getResultPtrFn.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("calling get_result_ptr: %w", err)
	}
	lenResults, err := getResultLenFn.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("calling get_result_len: %w", err)
	}

	resultPtr := uint32(ptrResults[0])
	resultLen := uint32(lenResults[0])

	resultBytes, ok := f.module.Memory().Read(resultPtr, resultLen)
	if !ok {
		return nil, fmt.Errorf("reading result from wasm memory")
	}

	var wr wasmResult
	if err := json.Unmarshal(resultBytes, &wr); err != nil {
		return nil, fmt.Errorf("parsing wasm result JSON: %w", err)
	}

	return &Violation{
		PolicyName: f.name,
		Action:     f.action,
		Message:    wr.Message,
	}, nil
}

func (f *WasmFilter) Close() error {
	ctx := context.Background()
	f.module.Close(ctx)
	return f.runtime.Close(ctx)
}
