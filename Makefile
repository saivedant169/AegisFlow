.PHONY: build run test lint vuln fmt fmt-check docker-build docker-up demo-local smoke e2e-approval-security e2e-installer e2e-cli benchmark clean

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

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...

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

demo-local:
	bash scripts/local_demo.sh

smoke:
	bash scripts/compose_smoke.sh

e2e-cli:
	python3 scripts/e2e_cli.py

e2e-installer:
	python3 scripts/e2e_installer.py

e2e-approval-security:
	bash scripts/e2e_approval_security.sh

benchmark:
	bash scripts/benchmark.sh

clean:
	rm -rf bin/
