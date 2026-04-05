.PHONY: build build-tinygo clean

build:
	GOOS=wasip1 GOARCH=wasm go build -o profanity-filter.wasm .

build-tinygo:
	tinygo build -o profanity-filter.wasm -target=wasip1 -no-debug .

clean:
	rm -f profanity-filter.wasm
