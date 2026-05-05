# Maintainers

## Current Maintainer

- Saivedant Hava - project owner and primary maintainer

## Maintainer Responsibilities

Maintainers are responsible for:

- keeping the default branch healthy
- reviewing security-sensitive changes carefully
- keeping demos, tests, and default examples free to run
- making releases and publishing release notes
- triaging issues and labeling beginner-friendly work
- keeping documentation accurate as behavior changes

## Project Rules

- Local demos, CI, examples, and default configs must not require paid services.
- Real providers must stay optional and bring-your-own-key.
- New defaults should prefer mock/local behavior unless production safety requires otherwise.
- Security, policy, auth, deployment, and release changes need tests or a clear validation note.
- Contributor-facing changes should use clear human project language without tool attribution.

## Branch Policy

External contributors should use pull requests. The project owner may push directly to `main` for small maintenance, release, documentation, and emergency fixes when the change has been tested locally or validated by GitHub Actions.

## Release Checklist

Before tagging a release:

- run `make fmt-check`
- run `go test ./... -race -count=1`
- run `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`
- run `bash scripts/compose_smoke.sh`
- verify Docker and release workflows on GitHub
- update `CHANGELOG.md`
- confirm README examples still work without paid services
