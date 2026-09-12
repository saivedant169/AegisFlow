# X thread

> Archived launch draft for v0.9.0. Do not publish as the development candidate announcement. Runtime upstream credential issuance is disabled; verify release artifacts and version-specific claims before reuse.


## 1

Coding agents call tools with real side effects. AegisFlow puts allow, review, or block policy on configured MCP and model paths before upstream execution.

v0.9.0 proof uses running local gateway, not staged UI.

https://github.com/saivedant169/AegisFlow

Attach: `docs/assets/hero-pr-writer.gif`

## 2

Recorded flow:

`github.list_repos` -> allow
`github.delete_repo` -> block
`github.create_pull_request` -> review
approve exact action -> retry allowed
signed evidence -> valid

## 3

Approval is single-use. Request ID and timestamp can change on retry. Repository, branch, title, actor, task, and arguments must match or policy asks for new review.

Attach: `docs/assets/shot-approval-queue.png`

## 4

v0.9.0 adds supported tool-call translation, per-session signed evidence, checksum verification, Sigstore bundle, SBOM, and build provenance.

Release: https://github.com/saivedant169/AegisFlow/releases/tag/v0.9.0

## 5

Apple M1, 30k requests, zero-latency mock provider:

55,327 req/s
0.6 ms p50
3.6 ms p99
0 errors

Method and 25 ms provider test: https://saivedant169.github.io/AegisFlow/performance/

Attach: `docs/assets/benchmark-card.png`

## 6

Boundary: AegisFlow governs traffic routed through it. It does not sandbox agent process or catch calls that bypass gateway.

Docs: https://saivedant169.github.io/AegisFlow/
