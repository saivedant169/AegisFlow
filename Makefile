.PHONY: build run test lint fmt fmt-check docker-build docker-up benchmark clean

BINARY=aegisflow
CONFIG=configs/aegisflow.yaml
GO_FILES=$(shell find . -name '*.go' -not -path './vendor/*')

build:
	go build -o bin/$(BINARY) ./cmd/aegisflow
	go build -o bin/aegisctl ./cmd/aegisctl

run: build
	./bin/$(BINARY) --config $(CONFIG)

test:
	go test ./... -v -race -count=1

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w $(GO_FILES)

fmt-check:
	@files="$$(gofmt -l $(GO_FILES))"; \
	if [ -n "$$files" ]; then \
		echo "Go files need formatting:"; \
		echo "$$files"; \
		exit 1; \
	fi

docker-build:
	docker build -t aegisflow:latest .

docker-up:
	docker compose -f deployments/docker-compose.yaml up --build

benchmark:
	bash scripts/benchmark.sh

clean:
	rm -rf bin/
