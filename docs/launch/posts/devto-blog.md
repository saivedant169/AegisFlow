---

> Archived launch draft for v0.9.0. Do not publish as the development candidate announcement. Runtime upstream credential issuance is disabled; verify release artifacts and version-specific claims before reuse.

title: Put policy between coding agent and its tools
published: false
description: A real allow, block, review, retry, and evidence flow with AegisFlow v0.9.0.
tags: security, go, mcp, opensource
---

# Put policy between coding agent and its tools

Coding agent with tool access is privileged client. It can read source, run commands, call APIs, and change remote state. Review after execution is too late for repository deletion or force push.

AegisFlow puts policy on configured protocol path. Routed call becomes `ActionEnvelope`:

```text
client -> MCP or model endpoint -> ActionEnvelope -> allow | review | block -> upstream
                                                     |
                                                     -> signed evidence
```

Policy can match actor, task, protocol, tool, target, arguments, and requested capability.

## Run local proof

No provider key or GitHub token is required.

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow/starter-kit
./install-pr-writer.sh
```

Installer builds services, starts local mock upstream, then checks allow, review, and block decisions.

![AegisFlow governed pull request flow](https://raw.githubusercontent.com/saivedant169/AegisFlow/v0.9.0/docs/assets/hero-pr-writer.gif)

## Block before execution

Recorded proof sends `github.delete_repo` through MCP gateway. Matching policy returns block. Client receives JSON-RPC `-32001`; mock upstream receives nothing.

![Blocked GitHub action](https://raw.githubusercontent.com/saivedant169/AegisFlow/v0.9.0/docs/assets/shot-blocked-action.png)

## Hold risky write for review

`github.create_pull_request` returns review-required error and enters admin queue.

![AegisFlow approval queue](https://raw.githubusercontent.com/saivedant169/AegisFlow/v0.9.0/docs/assets/shot-approval-queue.png)

Reviewer can approve from dashboard or CLI:

```bash
./bin/aegisctl pending
./bin/aegisctl approve <approval-id> "diff scope checked"
```

Client retries same action. Request ID and timestamp may change, but stable action fields must match. Changed repository, branch, title, actor, task, or arguments requires new review. Approval is consumed once.

## Verify evidence

Set stable key before starting gateway:

```bash
export AEGISFLOW_EVIDENCE_KEY=<secret-from-key-manager>
```

Each session gets separate signed hash chain.

Session registry is memory-only. Export evidence before gateway shutdown when retention matters. Stable key does not restore prior sessions.

```bash
./bin/aegisctl evidence sessions
./bin/aegisctl verify --session <session-id>
./bin/aegisctl evidence export <session-id> --file evidence.json
```

![Verified evidence](https://raw.githubusercontent.com/saivedant169/AegisFlow/v0.9.0/docs/assets/shot-evidence-verification.png)

Hash chain detects changed, deleted, or reordered records. It does not prove calls outside AegisFlow never happened. Signing key must stay outside repository.

## What v0.9.0 changes

Release adds supported tool-call translation for OpenAI-compatible, Anthropic, Gemini, and Ollama paths. Messages API tool passthrough is optional and disabled by default. Evidence and approvals are session scoped. GitHub App and AWS STS brokers can request task-specific access.

Release assets include checksums, Sigstore bundle, SPDX SBOM, and GitHub build provenance.

## Boundary

AegisFlow is gateway, not process sandbox. Calls that bypass configured endpoint are outside policy. Built-in editor file and shell tools are not intercepted automatically.

Repository: https://github.com/saivedant169/AegisFlow

Documentation: https://saivedant169.github.io/AegisFlow/
