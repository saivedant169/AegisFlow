package main

import (
	"encoding/json"
	"strings"
	"unsafe"
)

var resultBuf []byte

//export alloc
func alloc(size uint32) *byte {
	buf := make([]byte, size)
	return &buf[0]
}

//export check
func check(contentPtr *byte, contentLen uint32, metaPtr *byte, metaLen uint32) int32 {
	content := unsafe.String(contentPtr, contentLen)
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

//export get_result_ptr
func getResultPtr() *byte {
	if len(resultBuf) == 0 {
		return nil
	}
	return &resultBuf[0]
}

//export get_result_len
func getResultLen() uint32 {
	return uint32(len(resultBuf))
}

func main() {}
