// Profanity filter plugin for AegisFlow.
//
// Blocks requests that contain any word from a built-in deny list.
// This is a demonstration -- a production filter would use a more
// comprehensive word list and smarter matching (e.g., Levenshtein distance,
// Unicode normalization, leet-speak decoding).
//
// Build:
//
//	GOOS=wasip1 GOARCH=wasm go build -o profanity-filter.wasm .
package main

import (
	"strings"

	sdk "github.com/aegisflow/aegisflow/examples/wasm-plugin-sdk"
)

// denyList contains words that cause the request to be blocked.
// Extend this list for your use case.
var denyList = []string{
	"damn",
	"shit",
	"fuck",
	"ass",
	"bastard",
	"crap",
}

func init() {
	sdk.RegisterCheck(func(content string, meta sdk.Metadata) sdk.Result {
		lower := strings.ToLower(content)
		for _, word := range denyList {
			if strings.Contains(lower, word) {
				return sdk.BlockResult(
					"profanity detected: content contains blocked word \"" + word + "\"" +
						" (tenant: " + meta.TenantID + ", model: " + meta.Model + ")",
				)
			}
		}
		return sdk.PassResult()
	})
}

func main() {}
