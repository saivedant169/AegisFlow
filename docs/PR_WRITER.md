---
title: Governed pull request proof
description: Reproduce allow, block, review, approval, retry, and signed evidence with local AegisFlow services.
---

# Governed pull request proof

This walkthrough runs real AegisFlow MCP and admin endpoints against local mock GitHub server. No provider key, GitHub token, or paid service is used.

Recorded flow:

![AegisFlow governed pull request workflow](assets/hero-pr-writer.gif)

## What proof covers

| Step | Routed action | Decision or result |
|---|---|---|
| 1 | `github.list_repos` | allow, then forward upstream |
| 2 | `github.delete_repo` | block with JSON-RPC `-32001` |
| 3 | `github.create_pull_request` | review with JSON-RPC `-32002` |
| 4 | Reviewer approves pending item | approved |
| 5 | Client retries same pull request | allow, then forward upstream |
| 6 | Session evidence verification | valid signatures and chain |

Approval applies to one exact action. Request ID and timestamp may change during retry. Actor, task, protocol, tool, target, arguments, and requested capability must match.

## Run starter kit

Prerequisites: Go 1.26.6 or later, Node.js, `curl`, and `jq`.

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow/starter-kit
./install-pr-writer.sh
```

Installer performs these checks:

- Builds `aegisflow` and `aegisctl`.
- Starts gateway, admin API, MCP gateway, and mock MCP upstream.
- Confirms one allow, one review, and one block policy decision.

Local endpoints:

| Service | Address |
|---|---|
| Gateway | `http://127.0.0.1:8080` |
| Admin dashboard and API | `http://127.0.0.1:8081` |
| MCP gateway | `http://127.0.0.1:8082/mcp` |
| Mock MCP upstream | `http://127.0.0.1:3000` |

Generated local config is `configs/pr-writer.yaml`. Policy source is [`starter-kit/policies/pr-writer.yaml`](https://github.com/saivedant169/AegisFlow/blob/main/starter-kit/policies/pr-writer.yaml).

## Reproduce recorded proof

Run from repository root after starter kit is active:

```bash
./scripts/record-proof.sh
```

Script sends JSON-RPC calls through MCP gateway. It reads approval from admin queue, approves exact action through admin API, retries call, then verifies default evidence session.

Expected decision lines:

```text
github.list_repos                  ALLOW
github.delete_repo                 BLOCK
github.create_pull_request         REVIEW
reviewer: release-reviewer         APPROVED
github.create_pull_request         ALLOW
chain valid: true
evidence chain signatures verified
```

## Blocked action

Blocked call never reaches upstream mock server. MCP client receives structured error.

![Destructive GitHub action blocked by AegisFlow](assets/shot-blocked-action.png)

Relevant policy rule:

```yaml
- protocol: "mcp"
  tool: "github.delete_*"
  decision: "block"
```

## Approval queue

Pull request creation waits in admin queue. Reviewer can inspect tool, target, actor, and submission time before choosing approve or deny.

![Pull request action waiting in AegisFlow approval queue](assets/shot-approval-queue.png)

CLI can handle same queue:

```bash
./bin/aegisctl pending
./bin/aegisctl approve <approval-id> "diff scope checked"
```

Approval is consumed after matching retry. Changed title, branch, base, repository, actor, or task creates new review.

Restart and replay test uses same HTTP path and records each assertion:

![Approval replay E2E terminal run](assets/approval-security-e2e.gif)

```bash
make e2e-approval-security
```

## Signed evidence

Set stable evidence key before gateway starts:

```bash
export AEGISFLOW_EVIDENCE_KEY=<secret-from-key-manager>
```

Starter installer enables SQLite state under `.aegisflow-run/state.db` and keeps local evidence key in `.aegisflow-run/evidence.key`. For production, load key from secret manager. Without SQLite, approval and evidence state stays in memory.

```bash
./bin/aegisctl evidence sessions
./bin/aegisctl verify --session <session-id>
./bin/aegisctl evidence export <session-id> --file evidence.json
```

![AegisFlow signed evidence verification](assets/shot-evidence-verification.png)

Hash links and signatures detect changes to retained records. Detecting a removed tail or an omitted session requires an independently retained checkpoint or export. Verification does not prove calls outside AegisFlow never happened.

## Upstream credentials

Runtime upstream credential issuance is disabled in this development candidate. The local proof uses a mock upstream and mints no provider credentials. See [candidate migration](releases/corrective-candidate.md).

## Connect editor

- [Claude Code setup](https://github.com/saivedant169/AegisFlow/blob/main/starter-kit/editors/claude-code.md)
- [Cursor setup](https://github.com/saivedant169/AegisFlow/blob/main/starter-kit/editors/cursor.md)

Editor built-in file and shell actions are not intercepted automatically. Only calls routed through AegisFlow boundary enter policy engine.

## Stop local services

Installer writes process IDs under `.aegisflow-run/`:

```bash
kill "$(cat .aegisflow-run/aegisflow.pid)"
kill "$(cat .aegisflow-run/mock-mcp.pid)"
```

Read [threat model](security/THREAT_MODEL.md) before production use. Use [troubleshooting guide](troubleshooting.md) when local checks fail.
