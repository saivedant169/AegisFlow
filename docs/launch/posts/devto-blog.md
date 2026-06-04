---
title: "Governing a Coding Agent: A Step-by-Step Walkthrough of a Safe PR Writer"
published: false
tags: ai, security, devops, golang
---

Coding agents are good at real work now. They read a repo, run the tests, edit code, and open a pull request. The uncomfortable part is what sits behind that: the agent is holding credentials inside your perimeter and acting on its own. The question isn't *whether* to let it act — it's how to bound and prove what it did.

That's what AegisFlow does.

> AegisFlow lets coding agents read, test, edit, and open PRs safely — while blocking destructive actions, reviewing risky writes, minting scoped credentials, and proving what happened.

It's an Apache-2.0, single Go binary that runs locally with no paid cloud services for the core. It sits at the boundary between a coding agent and the tools it calls (MCP, shell, SQL, GitHub, HTTP). Every action is normalized into an `ActionEnvelope`, and a policy engine decides **allow / review / block**. The governance decision itself adds single-digit microseconds; the gateway sustains 58,000+ requests/sec at 1.1 ms p50.

This is pre-1.0 (v0.8.0), so treat it accordingly — but the workflow below is real and runs end to end.

## Install (under 10 seconds, no API keys)

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow/starter-kit
./install-pr-writer.sh
```

This installs the `pr-writer` policy pack, whose philosophy is *stop the scary stuff, stay out of the way*. Reads and tests are free; opening a PR is a review checkpoint; destructive shell is blocked. The default decision is `review`, so anything nobody anticipated fails closed, not open.

## The scenario

A coding agent is fixing a flaky test. It will read the codebase, run the tests, accidentally try to `rm -rf` a stale cache dir, draft a PR, wait for a human, get a scoped credential, and create the PR. At the end we export and verify the evidence. Each call below hits the admin test endpoint so you can replay the whole arc yourself.

### 1. Read the codebase (ALLOWED)

```bash
curl -s -X POST http://localhost:8081/admin/v1/test-action \
  -H "X-API-Key: starter-key-001" \
  -d '{"protocol":"git","tool":"github.get_file_contents","capability":"read"}' | jq .
```

```json
{ "decision": "allow", "matched_rule": "git:github.get_*", "policy": "pr-writer" }
```

Read-only, matches `github.get_*`, forwarded immediately.

### 2. Run the tests (ALLOWED)

```bash
curl -s -X POST http://localhost:8081/admin/v1/test-action \
  -H "X-API-Key: starter-key-001" \
  -d '{"protocol":"shell","tool":"shell.pytest","target":"/workspace","capability":"execute"}' | jq .
```

```json
{ "decision": "allow", "matched_rule": "shell:shell.pytest", "policy": "pr-writer" }
```

The agent gets `pytest` scoped to `/workspace` — not a shell. It can't pivot from running tests into running anything else, because every subsequent tool call comes back through AegisFlow as a fresh envelope.

### 3. The accidental `rm -rf` (BLOCKED)

The agent tries to clean up a `__pycache__` dir and generates `rm -rf /workspace`.

```bash
curl -s -X POST http://localhost:8081/admin/v1/test-action \
  -H "X-API-Key: starter-key-001" \
  -d '{"protocol":"shell","tool":"shell.rm","target":"/workspace","capability":"delete"}' | jq .
```

```json
{
  "decision": "block",
  "matched_rule": "shell:shell.rm",
  "error": {
    "code": -32001,
    "message": "policy: shell.rm is blocked by pr-writer policy",
    "data": { "reason": "destructive shell command" }
  }
}
```

Blocked at the protocol boundary, *before* execution — not detected afterward in a log. The agent sees a structured JSON-RPC `-32001` and self-corrects on the next turn. The block lands in the evidence chain just like an allow.

### 4. Draft the PR (REVIEW REQUIRED)

The fix is ready. The policy maps `github.create_pull_request` to `review`, so the action is parked in an approval queue and returns `-32002`:

```json
{
  "decision": "review",
  "error": {
    "code": -32002,
    "message": "approval required: action queued for human review"
  }
}
```

A human pulls the queue and sees the diff title, branch, and the agent's own justification before deciding.

### 5. Human approves

```bash
aegisctl approve d9e8a1b3-... \
  --reviewer alice \
  --comment "diff looks correct, scoped to the test file"
```

The reviewer's identity, timestamp, and comment are hash-linked into the evidence chain. Approval isn't a side note — it's part of the cryptographic record.

### 6. PR created with a scoped credential (ALLOWED)

The agent retries. AegisFlow sees the approval and — instead of handing over a standing token — mints a just-in-time GitHub App credential:

```json
{
  "decision": "allow",
  "credential": {
    "type": "github_app_jwt",
    "scope": "pull_requests:write,contents:read",
    "expires_in": 600
  },
  "result": { "pr_url": "https://github.com/.../pull/1247" }
}
```

Narrow scope, expires in 10 minutes, issued only because the policy allowed *and* a human approved. Revocation is automatic — it just expires.

### 7. Verify the evidence

```bash
aegisctl evidence verify --session sess-2026-04-06-1422
```

```
  allow   github.get_file_contents
  allow   shell.pytest
  block   shell.rm
  review  github.create_pull_request
  approve alice
  allow   github.create_pull_request
valid: true, total_entries: 7, audit log integrity verified
```

Seven SHA-256 hash-linked entries terminated by a session manifest hash. Edit the database after the fact and `verify` returns `valid: false` and points at the broken link. One exportable file answers "what did the agent do, who approved it, and can we trust the record."

## Why it matters

The agent got to do real work unattended. Three risky things were stopped or reviewed. And you have a single signed bundle to hand to security or compliance. Least privilege is just-in-time, destructive actions fail closed, and the audit is tamper-evident rather than log-scraping.

New in v0.8.0: point Claude Code or the Anthropic SDK at the gateway with `ANTHROPIC_BASE_URL=http://localhost:8080` and every prompt is policy-checked and audited before it reaches the provider — plus a Prometheus `aegisflow_policy_decisions_total{decision,protocol}` metric and a `sql-explorer` pack (SELECT allowed, writes reviewed, destructive SQL blocked).

**Full walkthrough:** https://github.com/saivedant169/AegisFlow/blob/main/docs/PR_WRITER.md

**Repo:** https://github.com/saivedant169/AegisFlow
