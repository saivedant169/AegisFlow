package main

import (
	"encoding/json"
	"strings"
	"unsafe"
)

var resultBuf []byte
var allocBufs [][]byte // keep references to prevent GC

//go:wasmexport alloc
func alloc(size uint32) uint32 {
	buf := make([]byte, size)
	allocBufs = append(allocBufs, buf)
	return uint32(uintptr(unsafe.Pointer(&buf[0])))
}

//go:wasmexport check
func check(contentPtr uint32, contentLen uint32, metaPtr uint32, metaLen uint32) int32 {
	content := unsafe.String((*byte)(unsafe.Pointer(uintptr(contentPtr))), contentLen)
	allocBufs = nil // free alloc'd buffers after reading
	if strings.Contains(strings.ToLower(content), "forbidden") {
		result, _ := json.Marshal(map[string]string{
			"action":  "block",
			"message": "content contains forbidden word",
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
