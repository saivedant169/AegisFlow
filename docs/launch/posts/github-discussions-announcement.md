# AegisFlow v0.8.0 — govern your coding agent's prompts, not just its tools

AegisFlow lets coding agents read, test, edit, and open PRs safely — while blocking destructive actions, reviewing risky writes, minting scoped credentials, and proving what happened.

v0.8.0 extends that governance boundary all the way up to the prompt itself: point Claude Code or the Anthropic SDK at AegisFlow and every request is policy-checked and audited **before** it reaches the provider.

- **Release:** https://github.com/saivedant169/AegisFlow/releases/tag/v0.8.0
- **The full proof, step by step:** https://github.com/saivedant169/AegisFlow/blob/main/docs/PR_WRITER.md

A quick honesty note: AegisFlow is pre-1.0. The core is solid and well-tested (~80% coverage), but APIs and policy formats can still shift before 1.0. Treat it accordingly, and tell us where it bites.

---

## What's new in v0.8.0

### Inbound Anthropic `/v1/messages` governance

Set one environment variable and your agent's traffic flows through AegisFlow first:

```
ANTHROPIC_BASE_URL=http://localhost:8080
```

Every prompt is normalized into an ActionEnvelope and run through the policy engine — **allow / review / block** — and recorded in the evidence chain *before* it leaves your machine for the provider. The gateway also supports `/v1/messages/count_tokens`. This is the same governance you already had at the MCP, shell, SQL, GitHub, and HTTP boundaries, now applied to the model call itself.

### Prometheus policy-decision metric

A new counter exposes exactly what the policy engine is deciding, broken down by outcome and protocol:

```
aegisflow_policy_decisions_total{decision,protocol}
```

Wire it into your existing Prometheus/Grafana setup to watch allow/review/block rates across MCP, shell, SQL, GitHub, HTTP, and now the Anthropic path.

### `sql-explorer` policy pack

A ready-to-use pack for governing database access: `SELECT` is allowed, writes are routed to human review, and destructive SQL is blocked. Drop it in as a starting point and tune from there.

### CLI: `--json` and `--dry-run`

`aegisctl` is friendlier to scripts and CI now:

- `--json` output on `status` and `pending` (with meaningful exit codes) so you can parse decisions and approval queues programmatically.
- `--dry-run` on `test-action` to preview what a policy *would* decide for an action without recording it.

---

## The one workflow this is all built around: the governed PR writer

A coding agent that drafts PRs, kept on a short leash:

1. **Reads** the repo — allowed (read-only).
2. **Runs tests** — allowed (scoped to the workspace, no shell).
3. Tries a destructive `rm -rf` cleanup — **blocked**.
4. **Drafts a PR** — routed to **human review**; a person reads the diff.
5. Human **approves** — reviewer identity is hash-linked into the record.
6. **PR opens** with a **scoped, 10-minute credential** — never the user's real token.
7. **Evidence exported and verified** — a hash-chained, tamper-evident chain you can independently check.

The governance decision itself adds single-digit microseconds. On the throughput side, the in-process governance pipeline benchmarks at ~58,000 evaluations/sec with ~1.1 ms p50 — overhead, not end-to-end HTTP throughput, so it stays out of the way.

Walkthrough with real envelopes, decisions, and log lines: https://github.com/saivedant169/AegisFlow/blob/main/docs/PR_WRITER.md

---

## Try it — no API keys, install-to-verified in under ~10 seconds

The starter kit uses a mock provider, so you can see the whole arc without signing up for anything:

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow/starter-kit
./install-pr-writer.sh
```

Then point your agent at the gateway to govern its prompts too:

```bash
export ANTHROPIC_BASE_URL=http://localhost:8080
# now run Claude Code or the Anthropic SDK as usual
```

AegisFlow is a single Go binary, local-first, and the core runs without any paid cloud services. Apache-2.0 licensed.

---

## Help shape what comes next

Two asks for this thread:

- **Share a policy pack.** Built a pack for your stack — a specific MCP server, a database shape, a GitHub workflow, an internal HTTP API? The `sql-explorer` and `pr-writer` packs are meant as starting points, not the ceiling. Post yours and we'll help others find it.
- **Report friction.** Where did a policy decision surprise you? Where was the CLI or the evidence export awkward? What did you expect `--dry-run` or the `/v1/messages` path to do that it didn't? Pre-1.0 is exactly the time to file these — drop a reply here or open an issue.

Thanks for kicking the tires.
