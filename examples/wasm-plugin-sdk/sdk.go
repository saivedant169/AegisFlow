// Package sdk provides helper functions for writing AegisFlow WASM policy plugins.
//
// Instead of manually managing memory pointers and JSON serialization,
// plugin authors import this package and write a simple Check function.
//
// Example usage:
//
//	package main
//
//	import sdk "github.com/aegisflow/aegisflow/examples/wasm-plugin-sdk"
//
//	func init() {
//	    sdk.RegisterCheck(func(content string, meta sdk.Metadata) sdk.Result {
//	        if containsBadWord(content) {
//	            return sdk.BlockResult("content blocked: bad word detected")
//	        }
//	        return sdk.PassResult()
//	    })
//	}
//
//	func main() {}
package sdk

import (
	"encoding/json"
	"unsafe"
)

// Metadata contains request context provided by the AegisFlow host.
type Metadata struct {
	TenantID string `json:"tenant_id"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
	Phase    string `json:"phase"`
}

// Result is the outcome of a policy check.
type Result struct {
	Block   bool
	Message string
}

// PassResult returns a Result that allows the request through.
func PassResult() Result {
	return Result{Block: false}
}

// BlockResult returns a Result that blocks the request with the given reason.
func BlockResult(message string) Result {
	return Result{Block: true, Message: message}
}

// CheckFunc is the signature for a plugin's policy check function.
// It receives the request content and metadata, and returns a Result.
type CheckFunc func(content string, meta Metadata) Result

// ---- internal state ----

var (
	registeredCheck CheckFunc
	resultBuf       []byte
	allocBufs       [][]byte
)

// RegisterCheck sets the plugin's check function. Call this once in an init()
// function. The registered function is invoked by the AegisFlow host whenever
// it needs to evaluate content against this plugin's policy.
func RegisterCheck(fn CheckFunc) {
	registeredCheck = fn
}

// ReadInput is a lower-level helper that converts a WASM pointer+length pair
// into a Go string. Most plugin authors do not need this -- use RegisterCheck
// instead. It is exported for advanced use cases where you need to read
// arbitrary host-provided memory.
func ReadInput(ptr uint32, length uint32) string {
	return unsafe.String((*byte)(unsafe.Pointer(uintptr(ptr))), length)
}

// ---- WASM exports required by the AegisFlow host ----
// These are automatically available when a plugin imports this package.
// The host calls alloc to write data into the module's memory, then calls
// check, then reads the result via get_result_ptr / get_result_len.

//go:wasmexport alloc
func wasmAlloc(size uint32) uint32 {
	buf := make([]byte, size)
	allocBufs = append(allocBufs, buf)
	return uint32(uintptr(unsafe.Pointer(&buf[0])))
}

//go:wasmexport check
func wasmCheck(contentPtr uint32, contentLen uint32, metaPtr uint32, metaLen uint32) int32 {
	content := ReadInput(contentPtr, contentLen)

	metaBytes := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(metaPtr))), metaLen)
	var meta Metadata
	_ = json.Unmarshal(metaBytes, &meta)

	// Free alloc'd buffers now that we have copied the data.
	allocBufs = nil

	if registeredCheck == nil {
		// No check registered -- default to pass.
		resultBuf = nil
		return 0
	}

	result := registeredCheck(content, meta)
	if !result.Block {
		resultBuf = nil
		return 0
	}

	resultJSON, _ := json.Marshal(map[string]string{
		"action":  "block",
		"message": result.Message,
	})
	resultBuf = resultJSON
	return 1
}

//go:wasmexport get_result_ptr
func wasmGetResultPtr() uint32 {
	if len(resultBuf) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&resultBuf[0])))
}

//go:wasmexport get_result_len
func wasmGetResultLen() uint32 {
	return uint32(len(resultBuf))
}
