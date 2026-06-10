# Let Coding Agents Draft PRs Safely

## The 30-second pitch

Your coding agent can read your repo, run tests, edit code, and open PRs. But it cannot merge to main, deploy to prod, touch destructive shell commands, or use broad credentials. Everything risky gets reviewed by a human. Everything destructive gets blocked. You have cryptographic proof of what happened.

AegisFlow sits between a coding agent and the tools it calls. One Go binary, one YAML policy file, one approval queue, one hash-chained evidence log.

---

## What this looks like in practice

Scenario: Claude Code is fixing a flaky test in `aegisflow/aegisflow`. It will read the codebase, run pytest, try (and fail) to clean up an artifact with `rm -rf`, draft a PR, get approved by a human, and finally land the PR. Every step flows through AegisFlow. At the end, we export and verify the evidence chain.

The policy in effect is `starter-kit/policies/pr-writer.yaml` (default decision: `review` — anything not explicitly allowed needs a human):

```bash
cd starter-kit && ./install-pr-writer.sh
```

### The whole arc at a glance

| # | Agent action | Tool | Decision | Why |
|---|--------------|------|----------|-----|
| 1 | Read the codebase | `github.get_file_contents` | **allow** | read-only |
| 2 | Run tests | `shell.pytest` | **allow** | scoped to `/workspace`, no shell |
| 3 | `rm -rf` cleanup | `shell.rm` | **block** | destructive shell |
| 4 | Draft a PR | `github.create_pull_request` | **review** | human reads the diff |
| 5 | Human approves | — | **approve** | reviewer identity hash-linked |
| 6 | PR created | `github.create_pull_request` | **allow** | scoped 10-min credential minted |
| 7 | Evidence exported + verified | — | **verify** | 7 hash-linked entries, integrity confirmed |

<!-- SCREENSHOT SLOT — replace with a capture of the admin approval queue at step 4
     (pending github.create_pull_request with diff title + justification visible).
     Recommended: docs/assets/proof-approval-queue.png
     ![AegisFlow approval queue holding a PR for review](assets/proof-approval-queue.png)
-->

---

### 1. Agent reads the codebase (ALLOWED)

Claude Code calls `github.get_file_contents` through the MCP bridge. AegisFlow normalizes it into an `ActionEnvelope`, matches `github.get_*` -> `allow`, and forwards it.

```bash
curl -s -X POST http://localhost:8081/admin/v1/test-action \
  -H "Content-Type: application/json" \
  -H "X-API-Key: starter-key-001" \
  -d '{"protocol":"git","tool":"github.get_file_contents","target":"aegisflow/aegisflow","capability":"read"}' | jq .
```

```json
{
  "envelope_id": "a7890917-44c6-44b0-8af4-283eb2933843",
  "decision": "allow",
  "matched_rule": "git:github.get_*",
  "policy": "pr-writer",
  "evidence_hash": "f3a1c8e9b2d4..."
}
```

AegisFlow log line:

```
{"ts":"2026-04-06T14:22:01Z","level":"info","msg":"action evaluated","envelope_id":"a7890917-44c6-44b0-8af4-283eb2933843","tool":"github.get_file_contents","decision":"allow","latency_us":1247}
```

---

### 2. Agent runs tests (ALLOWED)

```bash
curl -s -X POST http://localhost:8081/admin/v1/test-action \
  -H "Content-Type: application/json" \
  -H "X-API-Key: starter-key-001" \
  -d '{"protocol":"shell","tool":"shell.pytest","target":"/workspace","capability":"execute"}' | jq .
```

```json
{
  "envelope_id": "b1f0c324-0b9a-4e57-9c81-7e2a5d4f1c2b",
  "decision": "allow",
  "matched_rule": "shell:shell.pytest",
  "policy": "pr-writer",
  "evidence_hash": "9b2c4d6e8f10..."
}
```

The agent gets `pytest` on `/workspace` but does not get a shell. It cannot pivot from `pytest` to anything else: every subsequent tool call goes back through AegisFlow as a fresh envelope.

---

### 3. Agent tries to rm -rf the repo by mistake (BLOCKED)

Claude is trying to clean up a stale `__pycache__`. It generates `rm -rf /workspace`. AegisFlow rejects with JSON-RPC error `-32001` (policy block).

```bash
curl -s -X POST http://localhost:8081/admin/v1/test-action \
  -H "Content-Type: application/json" \
  -H "X-API-Key: starter-key-001" \
  -d '{"protocol":"shell","tool":"shell.rm","target":"/workspace","capability":"delete"}' | jq .
```

```json
{
  "envelope_id": "c4d7e2f1-8a3b-4c92-b6e0-1f5a8d9c3b47",
  "decision": "block",
  "matched_rule": "shell:shell.rm",
  "policy": "pr-writer",
  "error": {
    "code": -32001,
    "message": "policy: shell.rm is blocked by pr-writer policy",
    "data": {
      "reason": "destructive shell command",
      "suggested_alternative": "use git clean -fdx via shell.git"
    }
  }
}
```

The MCP bridge surfaces the `-32001` directly to Claude Code, which sees a structured error and self-corrects on the next turn. The block is recorded in the evidence chain just like an allow.

---

### 4. Agent drafts a PR (REVIEW REQUIRED)

The fix is ready. Claude calls `github.create_pull_request`. The PR-writer policy maps that to `review`. AegisFlow returns JSON-RPC error `-32002` (approval required) and parks the action in the approval queue.

```bash
curl -s -X POST http://localhost:8081/admin/v1/test-action \
  -H "Content-Type: application/json" \
  -H "X-API-Key: starter-key-001" \
  -d '{"protocol":"git","tool":"github.create_pull_request","target":"aegisflow/aegisflow","capability":"write"}' | jq .
```

```json
{
  "envelope_id": "d9e8a1b3-2c4f-4e6d-9a7b-5c3e8f1d2a4b",
  "decision": "review",
  "matched_rule": "git:github.create_pull_request",
  "policy": "pr-writer",
  "error": {
    "code": -32002,
    "message": "approval required: action queued for human review",
    "data": {
      "approval_url": "http://localhost:8081/admin/v1/approvals/d9e8a1b3-2c4f-4e6d-9a7b-5c3e8f1d2a4b",
      "queued_at": "2026-04-06T14:24:18Z"
    }
  }
}
```

Pending approvals queue:

```bash
curl -s http://localhost:8081/admin/v1/approvals -H "X-API-Key: starter-key-001" | jq .
```

```json
{
  "pending": [
    {
      "envelope_id": "d9e8a1b3-2c4f-4e6d-9a7b-5c3e8f1d2a4b",
      "actor": {"agent": "claude-code", "session": "sess-2026-04-06-1422"},
      "tool": "github.create_pull_request",
      "target": "aegisflow/aegisflow",
      "parameters": {
        "title": "fix: stabilize flaky TestEvidenceChain_Concurrent",
        "head": "claude/fix-flaky-evidence-test",
        "base": "main"
      },
      "queued_at": "2026-04-06T14:24:18Z",
      "justification": "fixes intermittent failure in TestEvidenceChain_Concurrent caused by unsynchronized map access"
    }
  ]
}
```

---

### 5. Human approves via CLI

```bash
aegisctl approve d9e8a1b3-2c4f-4e6d-9a7b-5c3e8f1d2a4b \
  --reviewer alice \
  --comment "diff looks correct, scoped to the test file"
```

```json
{
  "envelope_id": "d9e8a1b3-2c4f-4e6d-9a7b-5c3e8f1d2a4b",
  "status": "approved",
  "reviewer": "alice",
  "approved_at": "2026-04-06T14:25:42Z",
  "comment": "diff looks correct, scoped to the test file",
  "evidence_hash": "1c4e7a9b3d5f..."
}
```

The approval record is hash-linked into the evidence chain. Reviewer identity, timestamp, and comment are now part of the cryptographic record. Slack and the GraphQL admin API expose the same approval -- pick whichever fits your team.

---

### 6. Agent retries, PR is created (ALLOWED)

Claude Code retries the same tool call. AegisFlow sees the approval, issues a task-scoped GitHub App JWT (not the agent's standing token), and forwards the request.

```bash
curl -s -X POST http://localhost:8081/admin/v1/test-action \
  -H "Content-Type: application/json" \
  -H "X-API-Key: starter-key-001" \
  -d '{"protocol":"git","tool":"github.create_pull_request","target":"aegisflow/aegisflow","capability":"write","approval_ref":"d9e8a1b3-2c4f-4e6d-9a7b-5c3e8f1d2a4b"}' | jq .
```

```json
{
  "envelope_id": "e2a5b8c1-4d7e-4f0a-9b3c-6e8d1f4a7c2d",
  "decision": "allow",
  "matched_rule": "approved:d9e8a1b3-2c4f-4e6d-9a7b-5c3e8f1d2a4b",
  "credential": {
    "type": "github_app_jwt",
    "scope": "pull_requests:write,contents:read",
    "expires_in": 600
  },
  "result": {
    "pr_url": "https://github.com/aegisflow/aegisflow/pull/1247",
    "pr_number": 1247
  },
  "evidence_hash": "8d2f5a1c4b7e..."
}
```

Note the credential: `pull_requests:write,contents:read`, expires in 10 minutes. Not the agent's full token. Not your token. A just-in-time, narrow, expiring credential issued only because the policy allowed and a human approved.

---

### 7. Evidence exported and verified

Pull the full session bundle:

```bash
aegisctl evidence export --session sess-2026-04-06-1422 --out evidence.json
aegisctl evidence verify --session sess-2026-04-06-1422
```

```
verifying session sess-2026-04-06-1422...
  envelope a7890917-44c6-44b0-8af4-283eb2933843  allow   github.get_file_contents
  envelope b1f0c324-0b9a-4e57-9c81-7e2a5d4f1c2b  allow   shell.pytest
  envelope c4d7e2f1-8a3b-4c92-b6e0-1f5a8d9c3b47  block   shell.rm
  envelope d9e8a1b3-2c4f-4e6d-9a7b-5c3e8f1d2a4b  review  github.create_pull_request
  envelope d9e8a1b3-2c4f-4e6d-9a7b-5c3e8f1d2a4b  approve alice
  envelope e2a5b8c1-4d7e-4f0a-9b3c-6e8d1f4a7c2d  allow   github.create_pull_request
  envelope e2a5b8c1-4d7e-4f0a-9b3c-6e8d1f4a7c2d  result  pr/1247

valid: true, total_entries: 7, audit log integrity verified
session_manifest_hash: 4f8c2e1a9b3d6e0f7c5b8a2d4e6f1a3c9b5d8e0f2a4c6b8d1e3f5a7c9b2d4e6f
```

Seven entries, each one a SHA-256 hash linked to the previous, terminated by a session manifest hash. If anyone touches the database after the fact, `verify` returns `valid: false` and points at the broken link.

That's the whole story. The agent got to do real work. Three risky things were stopped or reviewed. You have a single file you can hand to compliance.

---

## What governance actually costs

Every allow, block, and review above passes through the full pipeline: envelope creation, policy evaluation, evidence chain record, and (on allow) credential minting. Micro-benchmarks on an Apple M1 (8GB RAM):

| Pipeline stage | p50 | p95 | Ops/sec |
|----------------|-----|-----|---------|
| Envelope creation | ~0.4 µs | ~0.5 µs | 2.5M+ |
| Policy evaluate — allow (20 rules) | ~1.2 µs | ~1.5 µs | 847K+ |
| Policy evaluate — block (no match) | ~0.7 µs | ~1.0 µs | 1.4M+ |
| Policy + evidence chain | ~3.4 µs | ~4.5 µs | 296K+ |
| Full allow (policy + evidence + credential) | ~5.2 µs | ~7.0 µs | 194K+ |
| Review path (policy + queue submit) | ~1.3 µs | ~1.8 µs | 779K+ |

The governance decision itself is single-digit microseconds. The full in-process pipeline (policy + evidence + credential) runs at ~58,000 evaluations/sec at 1.1 ms p50 — a single-threaded micro-benchmark, not end-to-end HTTP throughput. Reproduce locally:

```bash
./scripts/run_benchmarks.sh
go run ./scripts/benchmark_governance.go
```

---

## Why this matters for security and platform teams

Coding agents already have credentials inside your perimeter. The question is not *whether* to let them act, but *how to bound and prove* what they did.

- **Least privilege, just in time.** The agent never holds a standing GitHub token. Step 6 minted `pull_requests:write,contents:read` scoped to one action, expiring in 10 minutes. Revocation is automatic — it expires.
- **Human review only where it counts.** Reads and tests run unattended. PRs, merges, deployments, and branch-protection changes route to a human. The default decision is `review`, so a tool nobody anticipated fails closed, not open.
- **Destructive actions fail closed.** `rm -rf`, `github.delete_repo`, force pushes — blocked at the protocol boundary before execution, not detected after the fact in a log.
- **Tamper-evident audit, not log scraping.** The evidence chain is SHA-256 hash-linked and terminated by a session manifest hash. `aegisctl evidence verify` returns `valid: false` and points at the broken link if the record is altered. One exportable file answers "what did the agent do, who approved it, and can we trust the record."
- **Boundary-native, framework-agnostic.** Governance lives at the MCP/HTTP/shell/SQL/Git boundary, so it covers any agent or editor that speaks those protocols — no per-framework integration to maintain.

What an auditor or incident responder gets: one signed session bundle showing every attempted action, the policy decision, the approving human, the credential issued, and a cryptographic guarantee the record was not edited.

---

## Try it

**Install in under 3 minutes:**

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow/starter-kit
./install-pr-writer.sh
```

**Connect Claude Code:** see [`starter-kit/editors/claude-code.md`](../starter-kit/editors/claude-code.md)

**Read the policy:** [`starter-kit/policies/pr-writer.yaml`](../starter-kit/policies/pr-writer.yaml)

**See the code:** [github.com/saivedant169/AegisFlow](https://github.com/saivedant169/AegisFlow)
