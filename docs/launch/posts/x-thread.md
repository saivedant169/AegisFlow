**1/**
Coding agents are getting real tool access — shell, SQL, GitHub, HTTP. That's also how a single bad call drops a table, force-pushes, or leaks your token. Most setups just hope the agent behaves.

AegisFlow governs the boundary instead.

**2/**
It sits between the agent and the tools it calls. Every action — MCP, shell, SQL, GitHub, HTTP — is normalized into one ActionEnvelope a policy engine can reason about.

The governance decision adds single-digit microseconds. ~58k req/s, 1.1 ms p50.

**3/**
Three outcomes per action: allow, review, block.

Read the repo? allow. Open a PR? route to a human (review). `rm -rf` or destructive SQL? block.

You write the policy. The agent gets to work; the dangerous calls don't.

**4/**
New in v0.8.0: point Claude Code or the Anthropic SDK at the gateway —

ANTHROPIC_BASE_URL=http://localhost:8080

Now every prompt is policy-checked and audited *before* it reaches the provider. Same agent, governed path.

**5/**
Two more things it does instead of trusting blind:

- Mints a short-lived, task-scoped credential (e.g. 10 min) — not your real token.
- Records a hash-chained, tamper-evident evidence chain you can export and verify.

You can prove what happened.

**6/**
Apache-2.0, Go, single binary, local-first. Pre-1.0 and honest about it. No API keys to try it:

```
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow/starter-kit
./install-pr-writer.sh
```

The full story → https://github.com/saivedant169/AegisFlow/blob/main/docs/PR_WRITER.md

#AIagents
