# AegisFlow

AegisFlow is a local-first policy gateway for coding agents. Allow, review, or block routed MCP actions and verify signed session evidence.

[![CI](https://github.com/saivedant169/AegisFlow/actions/workflows/ci.yaml/badge.svg)](https://github.com/saivedant169/AegisFlow/actions/workflows/ci.yaml)
[![CodeQL](https://github.com/saivedant169/AegisFlow/actions/workflows/codeql.yml/badge.svg)](https://github.com/saivedant169/AegisFlow/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/saivedant169/AegisFlow/badge)](https://scorecard.dev/viewer/?uri=github.com/saivedant169/AegisFlow)
[![Go Report Card](https://goreportcard.com/badge/github.com/saivedant169/AegisFlow)](https://goreportcard.com/report/github.com/saivedant169/AegisFlow)
[![Go Reference](https://pkg.go.dev/badge/github.com/saivedant169/AegisFlow.svg)](https://pkg.go.dev/github.com/saivedant169/AegisFlow)
[![Release](https://img.shields.io/github/v/release/saivedant169/AegisFlow)](https://github.com/saivedant169/AegisFlow/releases/latest)
[![Docker pulls](https://img.shields.io/docker/pulls/saivedant169/aegisflow)](https://hub.docker.com/r/saivedant169/aegisflow)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

<p align="center">
  <img src="docs/assets/hero-pr-writer.gif" alt="AegisFlow allows a test, blocks destructive shell, queues a pull request for review, and verifies session evidence" width="900">
</p>

## Start here

Run a governed PR-writer policy locally. No provider key or paid service required.

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow/starter-kit
./install-pr-writer.sh
```

Installer builds AegisFlow, starts local gateway and admin services, loads PR-writer policy, and checks allow, review, and block decisions.

Connect an agent after checks pass:

- [Claude Code setup](starter-kit/editors/claude-code.md)
- [Cursor setup](starter-kit/editors/cursor.md)
- [PR-writer quickstart](starter-kit/QUICKSTART_PR_WRITER.md)
- [Full proof walkthrough](docs/PR_WRITER.md)

## What gets enforced

Routed MCP tool calls become an `ActionEnvelope`. Tool policy evaluates protocol, tool, target, capability, actor, task, and session context. Model API requests use input and output policy; they do not dispatch external tools. See [runtime support and verification](docs/runtime-support.md).

| Routed action | Example | Default PR-writer decision |
|---|---|---|
| Read repository content | `github.get_file_contents` | allow |
| Run tests | `shell.go_test` | allow |
| Delete files recursively | `shell.rm` | block |
| Open a pull request | `github.create_pull_request` | review |
| Merge or force push | `github.merge_pull_request` | block |
| Read database rows | `sql.select` | allow with `sql-explorer` |
| Change database rows | `sql.update` | review with `sql-explorer` |

Review decisions enter an approval queue. Optional upstream credential issuance is disabled in the development candidate pending broker hardening. See [candidate behavior and migration](docs/releases/corrective-candidate.md); published v0.9.0 has a different persistence and credential contract.

```text
MCP client -> authenticated gateway -> tool policy -> decision evidence
                                           |
                              allow -> upstream -> result evidence
                              review -> approval queue -> matching retry
                              block -> error response

Model API client -> input policy -> provider -> output policy -> response
```

## Boundary support

| Path | Governed behavior | Status |
|---|---|---|
| MCP remote gateway | `tools/list`, tool calls, arguments, review resume | supported |
| MCP stdio bridge | Routed tool calls and gateway failures | supported |
| OpenAI-compatible API | Input, output, streams, tool definitions, tool calls | supported |
| Anthropic Messages API | Input, output, streams, optional tool passthrough | supported |
| Built-in editor file or shell tools | Only covered when routed through MCP or another configured boundary | not intercepted directly |
| Standalone shell, SQL, GitHub, and HTTP gate libraries | Library behavior and unit tests | experimental, not wired |

`messages_api.tool_passthrough` remains off by default. Enable it only after testing selected provider tool loops.

## Install

### Verified release binary

```bash
curl -fsSL https://raw.githubusercontent.com/saivedant169/AegisFlow/v0.9.0/scripts/install.sh | AEGISFLOW_VERSION=v0.9.0 sh
```

Installer downloads `SHA256SUMS` from same release and rejects mismatched binaries.

### Homebrew

```bash
brew install saivedant169/tap/aegisflow
brew install saivedant169/tap/aegisctl
```

### Go

```bash
go install github.com/saivedant169/AegisFlow/cmd/aegisflow@v0.9.0
go install github.com/saivedant169/AegisFlow/cmd/aegisctl@v0.9.0
```

Source builds require Go 1.26.6 or later.

### Container

```bash
docker run --rm -p 8080:8080 -p 8081:8081 -p 8082:8082 \
  saivedant169/aegisflow:0.9.0
```

Same image also publishes at `ghcr.io/saivedant169/aegisflow:0.9.0`.

### Docker Compose

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow
docker compose -f deployments/docker-compose.yaml up --build
```

Gateway listens on `8080`, admin API and dashboard on `8081`, MCP gateway on `8082`.

## Policy packs

| Pack | Intended use | Read | Risky write | Destructive action |
|---|---|---:|---:|---:|
| `readonly` | Inspection and analysis | allow | block | block |
| `pr-writer` | Read, test, edit, open PR | allow | review | block |
| `docs-writer` | Documentation changes | allow | review | block |
| `infra-review` | Infrastructure planning | allow | review | block |
| `sql-explorer` | Data exploration | allow | review | block |

Policy files and tuning notes live under [`starter-kit/policies`](starter-kit/policies).

## Evidence and approvals

```bash
# Check gateway, admin API, providers, approvals, and evidence state
./bin/aegisctl status

# List pending actions
./bin/aegisctl pending

# Approve one exact action
./bin/aegisctl approve <action-id> "reviewed diff"

# Export and verify a session
./bin/aegisctl evidence export <session-id> --file evidence.json
./bin/aegisctl verify --session <session-id>
```

Memory storage remains default. Enable SQLite when approvals and evidence must survive restart:

```yaml
state:
  enabled: true
  sqlite_path: "data/aegisflow.db"
```

Set `AEGISFLOW_EVIDENCE_KEY` from a secret manager before starting. Same key authenticates approval rows and evidence records while staying outside SQLite. `AEGISFLOW_STATE_DB` can override configured path. SQLite mode supports one running AegisFlow instance; it is not shared state for multiple replicas.

Run restart, replay, changed-argument, and evidence-tamper checks with `make e2e-approval-security`.

## Release verification

v0.9.0 release includes checksums, Sigstore bundle, SPDX SBOM, and GitHub build provenance.

```bash
asset=aegisflow-linux-amd64
grep "  ${asset}$" SHA256SUMS | sha256sum --check -
cosign verify-blob \
  --bundle SHA256SUMS.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/saivedant169/AegisFlow/\.github/workflows/release\.yaml@refs/tags/v0\.9\.0$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  SHA256SUMS
gh attestation verify aegisflow-linux-amd64 \
  --repo saivedant169/AegisFlow
```

See [v0.9.0 release notes](docs/releases/v0.9.0.md) for upgrade steps and known limits.

## Documentation

| Need | Document |
|---|---|
| First local request | [Getting started](docs/getting-started.md) |
| Governed PR example | [PR-writer proof](docs/PR_WRITER.md) |
| Runtime components | [Architecture](docs/architecture.md) |
| API endpoints | [OpenAPI specification](api/openapi.yaml) |
| Configuration | [Annotated config](configs/aegisflow.example.yaml) |
| Deployment checks | [Production checklist](docs/production-checklist.md) |
| Incident response | [Operations runbook](docs/operations-runbook.md) |
| Metrics and traces | [Observability](docs/observability.md) |
| Performance method | [Performance](docs/performance.md) |
| Threat boundary | [Threat model](docs/security/THREAT_MODEL.md) |
| Provider and policy problems | [Troubleshooting](docs/troubleshooting.md) |

## Development

```bash
make build
make test
make vuln
make smoke
```

Current CI runs formatting, build, race tests, vulnerability checks, installer verification, Docker smoke tests, CodeQL, and OpenSSF Scorecard.

Contribution rules: [CONTRIBUTING.md](CONTRIBUTING.md). Security reports: [SECURITY.md](SECURITY.md). Questions and policy examples belong in [GitHub Discussions](https://github.com/saivedant169/AegisFlow/discussions).

## License

Apache License 2.0. See [LICENSE](LICENSE).
