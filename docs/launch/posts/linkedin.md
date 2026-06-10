Coding agents now run inside the perimeter. They read source, run tests, edit code, and open PRs — which means they hold credentials and reach for tools that can do real damage. An agent with your GitHub token isn't a code-completion feature; it's an unattended process making privileged calls. Most teams have no enforcement point between the agent's intent and the action it executes, and no record of what actually happened.

AegisFlow is runtime governance at that boundary — between a coding agent and the tools it calls (MCP, shell, SQL, GitHub, HTTP). Every action is normalized into a single envelope, and a policy engine decides allow / review / block before it runs.

What it enforces:

- Blocks destructive actions outright — e.g. `rm -rf` and other destructive shell never reach the host.
- Routes risky writes to human review — opening a PR holds in an approval queue so a person reads the diff before it lands.
- Mints short-lived, task-scoped credentials instead of handing the agent your token — the PR-writer flow uses a 10-minute scoped credential, not your standing access.
- Records a hash-chained, tamper-evident evidence chain you can export and verify — every decision is hash-linked, so you can prove what happened after the fact.
- Governs the prompt path too: point Claude Code or the Anthropic SDK at the gateway (`ANTHROPIC_BASE_URL=http://localhost:8080`) and every prompt is policy-checked and audited before it reaches the provider.

Operationally it's a single Go binary, local-first, Apache-2.0, and runs without paid cloud services for the core. The governance decision itself adds single-digit microseconds; the in-process governance pipeline runs ~58,000 evaluations/sec at ~1.1 ms p50 (a micro-benchmark, not end-to-end HTTP throughput), with roughly 80% test coverage. It's pre-1.0 (v0.8.0) — worth evaluating now, not yet a 1.0 guarantee.

The full governed PR-writer walkthrough — agent reads the repo, runs tests, is blocked from destructive shell, gets its PR routed to human review, receives a scoped credential, and the whole session comes out as verifiable evidence:

Proof: https://github.com/saivedant169/AegisFlow/blob/main/docs/PR_WRITER.md
Repo: https://github.com/saivedant169/AegisFlow
