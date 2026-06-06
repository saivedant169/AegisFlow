# Outreach templates

Short, honest, specific. You are asking for 30 minutes and real feedback, not
selling. No hype, no "revolutionary." Lead with their problem, not your repo.

Replace `<…>` placeholders. Personalize the first line every time — generic
outreach reads as spam and converts near zero.

---

## Cold email

**Subject:** governing what your coding agent can actually do

> Hi <name>,
>
> I saw <specific thing — your post about running Claude Code in CI / your MCP
> server / your comment on agent permissions>. That's exactly the problem I've
> been working on.
>
> I built AegisFlow — an open-source (Apache-2.0), local-first layer that sits
> between a coding agent and the tools it calls (shell, SQL, GitHub, MCP). It
> decides allow / review / block per action, mints short-lived scoped
> credentials instead of handing over your token, and keeps a verifiable record
> of what the agent did.
>
> I'm looking for a few people who actually run agents that take real actions to
> install it and tell me where it's wrong. 30 minutes, I mostly watch you
> install and react — no slides.
>
> Worth a look? Repo: https://github.com/saivedant169/AegisFlow
> The one-scenario walkthrough: https://github.com/saivedant169/AegisFlow/blob/main/docs/PR_WRITER.md
>
> — <you>

---

## DM / Slack / Discord (short)

> Hi <name> — saw <specific thing>. I'm building AegisFlow (OSS, Apache-2.0):
> runtime governance for coding agents — allow/review/block on shell, SQL,
> GitHub, MCP, with scoped credentials and a verifiable audit trail. Looking for
> a few people running real agents to install it and tell me where it breaks.
> 30 min, I just watch. Up for it? https://github.com/saivedant169/AegisFlow

---

## Reply when they say "sounds interesting"

> Great. Two ways to do it:
>
> 1. Async: install takes ~3 minutes, no API keys needed —
>    `git clone … && cd AegisFlow/starter-kit && ./install-pr-writer.sh`. Try it
>    against your agent and send me where you got stuck.
> 2. Live: 30 minutes on a call, you install while I watch and stay quiet. I
>    learn more from the live version — pick a slot: <calendar link>.
>
> Either way, the only ask is honesty about what's confusing or missing.

---

## Thank-you / follow-up after the call

> Thanks for the time, <name>. Most useful things you surfaced:
> - <friction 1> — filed as <issue link>
> - <friction 2> — filed as <issue link>
>
> I'll ping you when the first of these ships. If you'd like, I'll credit you in
> the release notes — say the word. And if anything else annoys you while using
> it, the install-problem template is the fastest way to reach me:
> https://github.com/saivedant169/AegisFlow/issues/new?template=install_problem.yml

---

## Tone rules

- Personalize line 1. If you can't, don't send it.
- Name the real numbers only if asked (58k req/s, 1.1 ms p50). Don't lead with them.
- Never claim users/stars you don't have.
- Make the ask small: 30 minutes, honesty, no commitment.
- Always include the proof link — it does the explaining for you.
