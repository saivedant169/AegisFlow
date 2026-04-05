# Makefile for building AegisFlow WASM plugins using the SDK.
#
# Usage from a plugin directory:
#   make build          -- compile to plugin.wasm using standard Go
#   make build-tinygo   -- compile to plugin.wasm using TinyGo (smaller binary)
#   make clean          -- remove built artifacts

PLUGIN_NAME ?= plugin
OUTPUT      ?= $(PLUGIN_NAME).wasm

.PHONY: build build-tinygo clean

# Build with the standard Go compiler (requires Go 1.24+).
build:
	GOOS=wasip1 GOARCH=wasm go build -o $(OUTPUT) .

# Build with TinyGo for a smaller binary.
# Requires TinyGo 0.34+ installed: https://tinygo.org/getting-started/install/
build-tinygo:
	tinygo build -o $(OUTPUT) -target=wasip1 -no-debug .

clean:
	rm -f $(OUTPUT)
