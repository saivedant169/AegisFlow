# Runtime support and verification

This contract describes development source after v0.9.0. It is not a promise about previously published binaries. See [candidate migration](releases/corrective-candidate.md) before upgrading.

## Supported boundaries

| Capability | Runtime path | Reproducible check | Limit |
|---|---|---|---|
| MCP authentication, session binding, tool policy, review retry | `internal/mcpgw/server.go` | `scripts/e2e_approval_security.sh`, `internal/mcpgw/server_test.go` | Only configured routed calls; requires trusted upstream metadata and appropriate rules |
| Caller roles and approval administration | `internal/admin/admin.go`, `internal/middleware/rbac.go` | `internal/admin/boundary_test.go`, `scripts/e2e_cli.py` | Admin access must be isolated; upstream writes are not transactional with local storage |
| Signed session evidence and optional restart persistence | `internal/evidence/chain.go`, `internal/evidence/sqlite_store.go` | `internal/evidence/persistence_test.go`, approval E2E | SQLite and a stable signing key are required for retained state |
| Model request and response policy, including streams | `internal/gateway/handler.go`, `internal/gateway/anthropic_messages.go` | `internal/gateway/stream_failure_test.go`, `internal/gateway/stream_scan_test.go`, container smoke | Model responses do not execute external tools; tool passthrough requires explicit configuration |
| CLI failure status and credential transport | `cmd/aegisctl/client.go` | `scripts/e2e_cli.py` | Local examples require explicit dry-run selection |
| Local starter setup and safe reinstall | `starter-kit/install-pr-writer.sh` | `scripts/e2e_installer.py` | Uses a local mock upstream; does not prove production provider behavior |
| Release artifact selection and verification | `scripts/install.sh`, `scripts/build_release.sh` | `scripts/e2e_release_install.py`, `scripts/test_install.sh` | Local fixtures do not validate hosted signing or publication |

Paths refer to the repository source. Run the named checks on the exact commit being evaluated; a list of tests is not a passing test report.

## Experimental or unavailable

Standalone shell, SQL, GitHub, HTTP, network-policy, and credential libraries are not automatically wired into the gateway's execution boundary. Their unit tests describe library behavior, not process isolation. Runtime upstream credential issuance rejects `credentials.enabled: true`.

Built-in editor file edits, terminal commands, and direct network calls bypass the gateway unless explicitly routed. An agent sharing the gateway's operating-system identity may read its local files or environment. Do not describe same-user deployment as secret isolation.

## Evidence and failure limits

A policy allow record is permission to dispatch, not proof of successful execution. Outcome recording can fail after an upstream side effect has happened. Treat such failures as uncertain execution and check upstream state before retrying writes.

Hash links and HMAC signatures protect retained records against changes without the signing key. They do not prove completeness of the entire session set, detect every removed tail without an independent checkpoint, or provide public-key non-repudiation. Protect the signing key and export evidence to independently retained storage.

Synthetic examples and local proof recordings are not customer incident reports. No adoption, prevented-loss, or production throughput claim follows from these tests.

## Release gate

Before selecting a new tag, require uncapped repository lint, race tests, CLI and approval E2E, starter installer E2E, strict documentation build, container smoke, release metadata validation, and native installation from the candidate's locally built release artifacts. Hosted signing, provenance, and publication remain a separate release gate. Check results for the exact candidate; do not reuse a prior commit's green CI as proof.
