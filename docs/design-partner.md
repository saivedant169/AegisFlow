# Design-partner program

A repeatable process for turning interest into real onboarding. The goal of a
design partner is not a logo — it is **one real install and honest friction
data** from someone who is not already inside your head.

What "good" looks like: 1–3 people install AegisFlow in front of a real coding
agent, you watch where they get stuck without helping too early, and every
meaningful friction point becomes an issue.

- Outreach templates (email + DM): [design-partner/outreach-templates.md](design-partner/outreach-templates.md)
- Target list + tracker: [design-partner/target-list.md](design-partner/target-list.md)

---

## Who you are looking for

A design partner is a fit if they already feel the pain. In priority order:

1. **Coding-agent users** — people running Claude Code, Cursor, Copilot agents, or homegrown agents that take actions (not just autocomplete).
2. **Platform / DevEx engineers** — own the tooling other engineers' agents run inside; care about blast radius.
3. **Security-minded OSS devs** — already think in terms of least privilege and audit.
4. **GitHub App / MCP builders** — already wiring agents to tools; understand the boundary problem immediately.

Not a fit (politely skip): people who only want autocomplete, anyone with no
agent taking real actions, and anyone who wants a managed SaaS today (AegisFlow
is local-first, pre-1.0).

---

## The 30-minute onboarding agenda

Keep it to 30 minutes. Your job is to **watch**, not to demo.

| Time | What | Note |
|------|------|------|
| 0–3 min | Frame: "I want to watch you install and react. Think aloud. I'll mostly stay quiet." | Set the watch-don't-help expectation. |
| 3–5 min | Ask the intake questions (below) before they touch anything. | Capture their mental model first. |
| 5–18 min | They run `git clone … && cd starter-kit && ./install-pr-writer.sh`, connect their agent, and try one real task. You stay silent unless they're fully stuck (>2 min). | This is the data. Note every hesitation. |
| 18–25 min | They try to make the agent do something risky (e.g. delete, force-push) and see it blocked/reviewed. | Does the block read as helpful or annoying? |
| 25–30 min | Debrief: what was confusing, what felt magic, what they'd need to use it for real. | Get the one thing they'd change. |

**Rule:** if they get stuck, wait. Count to 120 silently. The stall is the
finding. Help only after you've recorded where and why.

---

## Intake questions (ask before they install)

1. Which agent are you using? (Claude Code / Cursor / Copilot / custom / other)
2. What actions are you willing to let it take **today**, unsupervised?
3. What actions are you currently blocking **entirely** (or refusing to automate)?
4. What would make you trust an agent enough to let it open PRs?
5. What would be **unacceptable** friction — the thing that would make you uninstall?

Write the answers down verbatim. Question 3 and 5 are the most valuable.

---

## Post-call notes template

Copy this per session into your tracker or an issue.

```
Design partner: <name / handle>           Date: <YYYY-MM-DD>
Agent used: <Claude Code / Cursor / ...>   Role: <platform / security / IC / ...>

INTAKE
- Willing to allow today:
- Blocking entirely:
- Would trust PRs if:
- Unacceptable friction:

INSTALL (watch, don't help)
- Time to first governed action: <mm:ss>
- Got stuck at: <step> — for <how long> — because <why>
- "Aha" moment (if any):
- Quotes (verbatim):

RISKY-ACTION MOMENT
- Tried: <action>  ->  decision: <allow/review/block>
- Read as: helpful / annoying / confusing
- Reaction quote:

DEBRIEF
- One thing they'd change:
- Would they use it for real? <yes/no/conditional on ...>

FOLLOW-UPS (file each as an issue, tag external-feedback)
- [ ] issue: <title> (#)
- [ ] issue: <title> (#)
```

---

## After the call

1. File an issue for **every** meaningful friction point. Tag `external-feedback`.
2. Thank them; offer to credit them in release notes if they want.
3. Within a week, when you ship a fix they surfaced, tell them. That is what
   turns a design partner into an advocate.
4. Update the tracker row to `done` and link the issues.

The point of all of this: by the second install, it should go meaningfully
better than the first. If it doesn't, the friction data wasn't acted on.
