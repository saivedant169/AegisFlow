# Contributing to AegisFlow

Thank you for your interest in contributing to AegisFlow. This document provides guidelines for contributing.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/aegisflow.git`
3. Create a branch: `git checkout -b feature/your-feature`
4. Make your changes
5. Run tests: `make test`
6. Run linting: `make lint`
7. Commit and push
8. Open a Pull Request

## Development Setup

```bash
# Install Go 1.26.6 or later
brew install go

# Build
make build

# Run locally
make run

# Run tests
make test

# Run vulnerability checks
make vuln

# Run CLI authentication and failure checks against a local gateway
make e2e-cli

# Validate release metadata and installer failure handling
python3 scripts/test_release_metadata.py
bash scripts/test_install.sh

# Run the production-style smoke test
make smoke

# Run with Docker
make docker-up
```

CLI configuration and failure behavior: [CLI guide](docs/cli.md).

## Areas to Contribute

### Add a New Provider

1. Create `internal/provider/yourprovider.go`
2. Implement the `Provider` interface (6 methods)
3. Add tests in `internal/provider/yourprovider_test.go`
4. Register the type in `cmd/aegisflow/main.go` `initProviders()`
5. Add config support in `internal/config/config.go`

### Add a New Policy Filter

1. Create `internal/policy/filter_yourfilter.go`
2. Implement the `Filter` interface (3 methods)
3. Add tests
4. Register in `cmd/aegisflow/main.go` `initPolicyEngine()`

### Other Contributions

- Bug fixes
- Documentation improvements
- Performance optimizations
- Test coverage improvements

## Code Standards

- Follow standard Go conventions
- All exported functions must have tests
- Run `gofmt -s -w .` before committing
- Run `golangci-lint run ./...` and fix all issues
- Run `go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...`
- Keep packages small and focused

## Pull Request Process

1. Ensure all tests pass
2. Update documentation if needed
3. Keep PRs focused — one feature or fix per PR
4. Write a clear description of what changed and why

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
