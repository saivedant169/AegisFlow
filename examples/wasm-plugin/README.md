# AegisFlow WASM Policy Plugin Example

Reference implementation of an AegisFlow WASM policy plugin written in Go.

## Building

    make build

## Using

Copy `plugin.wasm` and add to your config:

    policies:
      input:
        - name: "custom-filter"
          type: "wasm"
          action: "block"
          path: "plugins/plugin.wasm"
          timeout: 100ms
          on_error: "block"

## ABI Contract

Your WASM module must export these functions:

| Export | Signature | Description |
|--------|-----------|-------------|
| alloc | (size i32) -> i32 | Allocate memory for host to write inputs |
| check | (content_ptr, content_len, meta_ptr, meta_len i32) -> i32 | Return 0 (allow) or 1 (violation) |
| get_result_ptr | () -> i32 | Pointer to result JSON after check returns 1 |
| get_result_len | () -> i32 | Length of result JSON |

### Metadata (input)

    {"tenant_id": "default", "model": "gpt-4o", "provider": "openai", "phase": "input"}

### Result (output on violation)

    {"action": "block", "message": "why this was flagged"}

The action in config always overrides the plugin's action.

## Using the SDK (recommended)

For new plugins, consider using the WASM Plugin SDK instead of implementing
the ABI by hand. The SDK handles memory management and JSON serialization
for you:

    examples/wasm-plugin-sdk/   -- the SDK package
    examples/wasm-plugins/      -- example plugins built with the SDK
    docs/wasm-plugin-guide.md   -- full tutorial and ABI reference

## Supported Languages

Any language compiling to WASM/WASI: Go, TinyGo, Rust, AssemblyScript.
