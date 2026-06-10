**Coding agents now take real actions and hold real credentials. We're treating that like a security boundary problem.**

A year ago "AI in the loop" mostly meant autocomplete. Now I've got agents that read my repo, run my test suite, edit files, and open PRs. Which is great, until you sit with what that actually means: the agent is executing shell, hitting GitHub's API, running SQL, and to do any of it I've usually handed it a token with my full access. There's no real boundary between "read a file" and "force-push to main" — it's the same credential and the same trust.

That's the gap I've been working on. The project is **AegisFlow** (Apache-2.0, Go, single binary, local-first — the core runs with no paid cloud services).

**What it does**

It sits at the boundary between a coding agent and the tools it calls — MCP, shell, SQL, GitHub, HTTP. Every action gets normalized into a single ActionEnvelope, and a policy engine decides one of three things: **allow / review / block**. On top of that it does two things I care about a lot:

- Instead of handing the agent your token, it **mints short-lived, task-scoped credentials** (think a 10-minute scoped cred for one job).
- It records a **hash-chained, tamper-evident evidence log** you can export and verify after the fact, so you can actually prove what the agent did.

The governance decision itself adds single-digit microseconds; the in-process governance pipeline benchmarks at ~58,000 evaluations/sec, ~1.1 ms p50 (not end-to-end HTTP throughput). Test coverage is around 80%. Being upfront: it's pre-1.0, so treat it accordingly.

**The example I'd point at: a governed PR-writer**

This is the workflow I built everything around. Claude Code is fixing a flaky test in a repo, and the whole session runs through AegisFlow:

1. Reads the codebase → **allowed** (read-only)
2. Runs tests → **allowed** (scoped to the workspace)
3. Tries `rm -rf` to clean up → **blocked** (destructive shell)
4. Drafts a PR → **routed to human review** (a person reads the diff)
5. Human approves → PR opens, with a **scoped 10-minute credential** minted just for that
6. The whole session comes out as a verifiable, hash-linked evidence chain

So the agent can do the useful stuff autonomously, the risky write waits for a human, the destructive thing never happens, and afterward you can verify exactly what occurred.

There's also a newer piece: you can point Claude Code or the Anthropic SDK at the gateway with `ANTHROPIC_BASE_URL=http://localhost:8080` and every prompt gets policy-checked and audited *before* it reaches the provider. Plus a Prometheus metric for policy decisions (`aegisflow_policy_decisions_total{decision,protocol}`) and a sql-explorer policy pack (SELECT allowed, writes reviewed, destructive SQL blocked).

**Try it (no API keys, uses a mock provider, install-to-verified in under ~10s):**

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow/starter-kit
./install-pr-writer.sh
```

Repo: https://github.com/saivedant169/AegisFlow
Full walkthrough of the PR-writer (the whole arc, step by step): https://github.com/saivedant169/AegisFlow/blob/main/docs/PR_WRITER.md

**The ask:** I'd genuinely like feedback from people running agents against real infrastructure. Is allow/review/block the right model, or too coarse? Does scoped-credential-minting fit how you actually hand out access, or does it break your existing auth flow? And is a hash-chained evidence log something you'd actually use for audit, or is that solving a problem you don't have yet? Tell me where this is wrong — that's more useful to me right now than where it's right.
