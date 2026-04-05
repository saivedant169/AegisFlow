# AegisFlow WASM Plugin Guide

This guide walks you through creating, building, testing, and deploying custom
policy plugins for AegisFlow using WebAssembly (WASM).

## Overview

AegisFlow evaluates every LLM request and response against a chain of policy
filters. Filters can be built-in (keyword matching, regex) or custom WASM
modules. WASM plugins let you ship arbitrary policy logic as a single `.wasm`
file without recompiling AegisFlow itself.

The host runtime is [wazero](https://wazero.io/) -- a pure-Go WASM engine with
zero CGO dependencies. Plugins run in a sandboxed environment with no access to
the network, filesystem, or host memory outside their own linear memory.

### How it works

1. AegisFlow loads your `.wasm` file at startup.
2. For each request (or response), the host:
   - Calls `alloc` twice to reserve space in the module's memory.
   - Writes the content string and a JSON metadata blob into those buffers.
   - Calls `check(content_ptr, content_len, meta_ptr, meta_len)`.
   - If `check` returns `1`, the host reads the result JSON via
     `get_result_ptr` / `get_result_len`.
3. The configured `action` (block, warn, log) is applied.

## Prerequisites

- **Go 1.24 or later** (for `GOOS=wasip1 GOARCH=wasm` support).  
  Go 1.24 added `//go:wasmexport` which this SDK uses.
- **(Optional) TinyGo 0.34+** for smaller binaries.  
  Install: <https://tinygo.org/getting-started/install/>

No CGO, Docker, or special toolchain is needed beyond the Go compiler.

## Quick start: your first plugin in 5 minutes

### 1. Create a new module

```bash
mkdir my-plugin && cd my-plugin
go mod init my-org/my-plugin

# Add the SDK as a dependency.
# If you cloned the AegisFlow repo, use a replace directive:
go mod edit -require github.com/aegisflow/aegisflow/examples/wasm-plugin-sdk@v0.0.0
go mod edit -replace github.com/aegisflow/aegisflow/examples/wasm-plugin-sdk=../path/to/AegisFlow/examples/wasm-plugin-sdk
```

### 2. Write the plugin

Create `main.go`:

```go
package main

import (
    "strings"

    sdk "github.com/aegisflow/aegisflow/examples/wasm-plugin-sdk"
)

func init() {
    sdk.RegisterCheck(func(content string, meta sdk.Metadata) sdk.Result {
        if strings.Contains(strings.ToLower(content), "password") {
            return sdk.BlockResult("content mentions a password")
        }
        return sdk.PassResult()
    })
}

func main() {}
```

Key points:
- Register your check in `init()`, not `main()`.
- `main()` must exist but should be empty.
- The SDK handles all WASM exports (`alloc`, `check`, `get_result_ptr`,
  `get_result_len`) automatically.

### 3. Build to WASM

```bash
GOOS=wasip1 GOARCH=wasm go build -o my-plugin.wasm .
```

Or with TinyGo for a smaller binary (~100KB vs ~2MB):

```bash
tinygo build -o my-plugin.wasm -target=wasip1 -no-debug .
```

### 4. Deploy

Copy the `.wasm` file to your AegisFlow server and add it to your config:

```yaml
policies:
  input:
    - name: "my-plugin"
      type: "wasm"
      action: "block"
      path: "plugins/my-plugin.wasm"
      timeout: 100ms
      on_error: "block"
```

### 5. Test

Send a request through AegisFlow and confirm the plugin fires:

```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"what is my password"}]}'
```

You should get a policy violation response.

## SDK reference

### Types

```go
// Metadata is the request context from the AegisFlow host.
type Metadata struct {
    TenantID string `json:"tenant_id"`
    Model    string `json:"model"`
    Provider string `json:"provider"`
    Phase    string `json:"phase"`   // "input" or "output"
}

// Result is what your check function returns.
type Result struct {
    Block   bool
    Message string
}
```

### Functions

| Function | Description |
|----------|-------------|
| `RegisterCheck(fn CheckFunc)` | Register your policy check. Call once in `init()`. |
| `PassResult() Result` | Return this to allow the request. |
| `BlockResult(msg string) Result` | Return this to flag a violation with a reason. |
| `ReadInput(ptr, len uint32) string` | (Advanced) Read raw memory. Most plugins do not need this. |

### CheckFunc signature

```go
type CheckFunc func(content string, meta Metadata) Result
```

- `content` -- the request or response text being evaluated.
- `meta` -- context about the request (tenant, model, provider, phase).
- Return `PassResult()` or `BlockResult("reason")`.

## ABI reference (low-level)

If you are writing a plugin in Rust, AssemblyScript, or another language
that cannot import the Go SDK, you need to implement these four exports:

| Export | Signature (WASM types) | Description |
|--------|------------------------|-------------|
| `alloc` | `(size: i32) -> i32` | Allocate `size` bytes in module memory. Return the pointer. |
| `check` | `(content_ptr: i32, content_len: i32, meta_ptr: i32, meta_len: i32) -> i32` | Evaluate the content. Return `0` to allow, `1` to block. |
| `get_result_ptr` | `() -> i32` | After `check` returns `1`, return a pointer to the result JSON. |
| `get_result_len` | `() -> i32` | Length (bytes) of the result JSON. |

### Memory layout

```
Module linear memory
+------+---------------------------------------------+
| addr | content                                     |
+------+---------------------------------------------+
| 0x.. | [content bytes written by host via alloc]    |
| 0x.. | [metadata JSON written by host via alloc]    |
| 0x.. | [result JSON written by plugin after check]  |
+------+---------------------------------------------+
```

The host performs these steps in order:

1. Call `alloc(content_len)` -- returns `content_ptr`.
2. Write `content` bytes to `[content_ptr .. content_ptr+content_len)`.
3. Call `alloc(meta_len)` -- returns `meta_ptr`.
4. Write metadata JSON to `[meta_ptr .. meta_ptr+meta_len)`.
5. Call `check(content_ptr, content_len, meta_ptr, meta_len)`.
6. If result is `1`:
   - Call `get_result_ptr()` and `get_result_len()`.
   - Read `result_len` bytes starting at `result_ptr`.

### Metadata JSON (input to check)

```json
{
  "tenant_id": "default",
  "model": "gpt-4o",
  "provider": "openai",
  "phase": "input"
}
```

`phase` is `"input"` for request policies and `"output"` for response policies.

### Result JSON (output on violation)

```json
{
  "action": "block",
  "message": "human-readable reason the content was flagged"
}
```

The `action` field in the result JSON is informational only. The action
configured in `aegisflow.yaml` always takes precedence.

## Configuration reference

Add a WASM policy under the `policies.input` or `policies.output` section of
your AegisFlow config file (usually `aegisflow.yaml`):

```yaml
policies:
  input:
    - name: "profanity-filter"
      type: "wasm"
      action: "block"        # block | warn | log
      path: "plugins/profanity-filter.wasm"
      timeout: 100ms         # max execution time per check (default: 100ms)
      on_error: "block"      # what to do if the plugin crashes: block | allow
  output:
    - name: "json-validator"
      type: "wasm"
      action: "block"
      path: "plugins/json-validator.wasm"
      timeout: 200ms
      on_error: "allow"
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `name` | yes | -- | Unique identifier for this policy. |
| `type` | yes | -- | Must be `"wasm"`. |
| `action` | yes | -- | Action on violation: `block`, `warn`, or `log`. |
| `path` | yes | -- | Path to the `.wasm` file (relative to working dir or absolute). |
| `timeout` | no | `100ms` | Maximum time the plugin may run per invocation. |
| `on_error` | no | `block` | Behavior when the plugin errors or times out. |

## Installing plugins from the marketplace

You can also install community plugins directly:

```bash
# Search for plugins
aegisctl plugin search profanity

# Get details
aegisctl plugin info profanity-filter

# Install (downloads .wasm, verifies SHA-256, updates plugins.yaml)
aegisctl plugin install profanity-filter

# List installed plugins
aegisctl plugin list

# Check for updates
aegisctl plugin outdated

# Remove a plugin
aegisctl plugin remove profanity-filter
```

## Example plugins

The repository includes two ready-to-use example plugins built with the SDK:

### Profanity filter

Location: `examples/wasm-plugins/profanity-filter/`

Blocks content containing common profane words. Demonstrates basic string
matching with the SDK.

```bash
cd examples/wasm-plugins/profanity-filter
make build
# => profanity-filter.wasm
```

### JSON validator

Location: `examples/wasm-plugins/json-validator/`

Blocks content that is not valid JSON. Useful as an output policy to enforce
structured LLM responses.

```bash
cd examples/wasm-plugins/json-validator
make build
# => json-validator.wasm
```

## Debugging tips

### Plugin does not load

- Verify the file path in your config is correct (try an absolute path).
- Check that all four exports exist. Run:
  ```bash
  wasm-tools print my-plugin.wasm | grep "export"
  ```
  You should see `alloc`, `check`, `get_result_ptr`, `get_result_len`.
- If using TinyGo, make sure you are targeting `wasip1`:
  ```bash
  tinygo build -target=wasip1 ...
  ```

### Plugin times out

- The default timeout is 100ms. Increase it in the config if your logic
  is legitimately slow.
- Avoid unbounded loops or large allocations in your check function.
- Profile with `GODEBUG=gctrace=1` if you suspect GC pressure.

### Plugin returns wrong result

- Print your result JSON to stderr during development. wazero passes stderr
  through to the host's stderr by default.
- Verify your result JSON matches the expected schema:
  `{"action": "block", "message": "..."}`.
- Remember: the `action` in config overrides whatever your plugin returns.
  If config says `warn` but your JSON says `block`, the host uses `warn`.

### Inspecting the WASM binary

```bash
# List exports
wasm-tools print plugin.wasm | grep export

# Disassemble to WAT (text format)
wasm-tools print plugin.wasm > plugin.wat

# Check binary size
ls -lh plugin.wasm
```

### Logging from within a plugin

The standard Go `println()` and `fmt.Fprintf(os.Stderr, ...)` write to stderr
in WASI modules. wazero routes module stderr to the host process stderr, so
these messages appear in AegisFlow's log output. Use them freely during
development; remove or guard them for production to avoid noise.

## Limitations and known issues

- **No networking.** WASM plugins run in a sandbox with no socket access.
  If your policy needs to call an external API, use a native Go filter instead.
- **No filesystem access.** Plugins cannot read files from the host. Pass all
  data through the `content` and `metadata` parameters.
- **Single-threaded execution.** The host holds a mutex during `check` calls,
  so a single WASM module instance handles one request at a time. For high
  throughput, consider running multiple AegisFlow replicas.
- **Memory limit.** wazero enforces a default linear memory limit. Very large
  payloads (>10MB) may cause allocation failures.
- **No state between calls.** Global variables persist between calls to the
  same module instance, but you should not rely on this for correctness.
  Module instances may be recycled.
- **Go standard library size.** Plugins built with standard Go produce ~2MB
  `.wasm` files due to the runtime. Use TinyGo to get ~100KB binaries, but
  note that TinyGo has limited standard library support (e.g., no `reflect`,
  limited `encoding/json`).
- **`//go:wasmexport` requires Go 1.24+.** Earlier Go versions do not support
  this directive.
