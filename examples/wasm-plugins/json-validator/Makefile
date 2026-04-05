.PHONY: build build-tinygo clean

build:
	GOOS=wasip1 GOARCH=wasm go build -o json-validator.wasm .

build-tinygo:
	tinygo build -o json-validator.wasm -target=wasip1 -no-debug .

clean:
	rm -f json-validator.wasm
