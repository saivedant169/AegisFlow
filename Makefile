.PHONY: build run test lint fmt docker-build docker-up clean

BINARY=aegisflow
CONFIG=configs/aegisflow.yaml

build:
	go build -o bin/$(BINARY) ./cmd/aegisflow

run: build
	./bin/$(BINARY) --config $(CONFIG)

test:
	go test ./... -v -race -count=1

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .

docker-build:
	docker build -t aegisflow:latest .

docker-up:
	docker compose -f deployments/docker-compose.yaml up --build

clean:
	rm -rf bin/
