.PHONY: build clean

build:
	GOOS=wasip1 GOARCH=wasm go build -o plugin.wasm .

clean:
	rm -f plugin.wasm
