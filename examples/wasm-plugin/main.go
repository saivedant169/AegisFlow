// Example AegisFlow WASM policy plugin.
// Blocks any message containing the word "forbidden".
// Build: GOOS=wasip1 GOARCH=wasm go build -o plugin.wasm .
package main

import (
	"encoding/json"
	"strings"
	"unsafe"
)

var resultBuf []byte
var allocBufs [][]byte // keep references to prevent GC

type metadata struct {
	TenantID string `json:"tenant_id"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
	Phase    string `json:"phase"`
}

//go:wasmexport alloc
func alloc(size uint32) uint32 {
	buf := make([]byte, size)
	allocBufs = append(allocBufs, buf)
	return uint32(uintptr(unsafe.Pointer(&buf[0])))
}

//go:wasmexport check
func check(contentPtr uint32, contentLen uint32, metaPtr uint32, metaLen uint32) int32 {
	content := unsafe.String((*byte)(unsafe.Pointer(uintptr(contentPtr))), contentLen)

	metaBytes := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(metaPtr))), metaLen)
	var meta metadata
	_ = json.Unmarshal(metaBytes, &meta)

	allocBufs = nil // free alloc'd buffers after reading

	if strings.Contains(strings.ToLower(content), "forbidden") {
		result, _ := json.Marshal(map[string]string{
			"action":  "block",
			"message": "content contains forbidden word (tenant: " + meta.TenantID + ", model: " + meta.Model + ")",
		})
		resultBuf = result
		return 1
	}

	return 0
}

//go:wasmexport get_result_ptr
func getResultPtr() uint32 {
	if len(resultBuf) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(&resultBuf[0])))
}

//go:wasmexport get_result_len
func getResultLen() uint32 {
	return uint32(len(resultBuf))
}

func main() {}
