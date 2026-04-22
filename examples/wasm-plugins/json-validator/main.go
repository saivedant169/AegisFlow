// JSON validator plugin for AegisFlow.
//
// Blocks requests whose content is not valid JSON. Useful on output policies
// where you need structured responses from an LLM, or on input policies
// where a downstream tool expects JSON payloads.
//
// Build:
//
//	GOOS=wasip1 GOARCH=wasm go build -o json-validator.wasm .
package main

import (
	"encoding/json"
	"fmt"
	"strings"

	sdk "github.com/aegisflow/aegisflow/examples/wasm-plugin-sdk"
)

func init() {
	sdk.RegisterCheck(func(content string, meta sdk.Metadata) sdk.Result {
		trimmed := strings.TrimSpace(content)

		// Empty content is not valid JSON.
		if len(trimmed) == 0 {
			return sdk.BlockResult("json-validator: empty content is not valid JSON")
		}

		// Try to parse the content as JSON.
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return sdk.BlockResult(
				fmt.Sprintf("json-validator: invalid JSON: %s (tenant: %s, phase: %s)",
					err.Error(), meta.TenantID, meta.Phase),
			)
		}

		return sdk.PassResult()
	})
}

func main() {}
