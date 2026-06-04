**Show HN: AegisFlow – Policy and audit boundary for coding agents (Go, Apache-2.0)**

AegisFlow is a single Go binary that sits between a coding agent (or any tool-using agent) and the tools it calls — MCP, shell, SQL, GitHub, HTTP. Every action is normalized into one struct (an `ActionEnvelope`) and the policy engine returns one of three decisions: allow, review, or block. It's local-first and runs without any paid cloud service for the core.

**Why I built it.** Coding agents already run inside the perimeter with real credentials. They can read repos, run tests, edit code, and open PRs. The problem isn't whether to let them act, it's how to bound what they can do and prove afterward what they did. Logging after the fact doesn't stop an `rm -rf` or a force push. Handing the agent a standing GitHub token means it holds far more access than any single task needs. I wanted the decision to happen at the boundary, before execution, with a record you can verify.

**How it works.** A request comes in (MCP tool call, shell command, SQL statement, HTTP call). AegisFlow normalizes it into an envelope and evaluates a YAML policy. Default decision is `review`, so anything not explicitly allowed fails closed to a human, not open. Reads and tests are allowed unattended. Destructive shell (`rm -rf`, force push, `github.delete_repo`) is blocked. A PR open routes to an approval queue; a human approves via CLI or admin API, and only then does AegisFlow mint a short-lived, task-scoped credential (e.g. `pull_requests:write,contents:read`, 10-minute expiry) instead of passing through the user's token. Every allow, block, review, and approval is written to a SHA-256 hash-chained evidence log terminated by a session manifest hash. `aegisctl evidence verify` returns `valid: false` and points at the broken link if the database was edited after the fact.

The workflow I'd point at is the governed PR writer: agent reads the repo, runs the tests, gets blocked from destructive shell, has its PR open held for review, gets a scoped 10-minute credential after approval, and the whole session exports as one verifiable bundle. Walkthrough with the actual JSON-RPC responses: https://github.com/saivedant169/AegisFlow/blob/main/docs/PR_WRITER.md

**Numbers.** On an M1 the governance decision itself is single-digit microseconds. End to end the gateway sustains ~58,000 req/s at 1.1 ms p50. About 80% test coverage. Benchmarks are reproducible with the scripts in the repo.

**What it does NOT do yet (it's pre-1.0):**
- The Anthropic path (point Claude Code or the SDK at `ANTHROPIC_BASE_URL=http://localhost:8080` so prompts are policy-checked and audited before reaching the provider) does NOT support tool-use passthrough yet — it governs the prompt/completion path, not in-flight tool calls on that route.
- It's pre-1.0. APIs, policy schema, and the evidence format can still change between releases.
- Credential minting beyond the GitHub App path is limited; other providers need more work.
- It enforces at the protocols it understands (MCP, shell, SQL, Git, HTTP). It does not sandbox the agent's process or stop something that bypasses the boundary entirely.

Apache-2.0. Install with no API keys (mock provider): `git clone`, `cd AegisFlow/starter-kit`, `./install-pr-writer.sh`.

Repo: https://github.com/saivedant169/AegisFlow
