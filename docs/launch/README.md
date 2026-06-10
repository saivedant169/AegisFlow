# Launch content bundle

Everything needed to launch AegisFlow v0.8.0 in multiple places without
rewriting the story each time. Posts are drafts — review before posting.

## One-line pitch (use everywhere)

> AegisFlow lets coding agents read, test, edit, and open PRs safely — while blocking destructive actions, reviewing risky writes, minting scoped credentials, and proving what happened.

## The one workflow everything centers on

The governed coding-agent PR writer: the agent reads the repo, runs tests, is
blocked from destructive shell, has its PR open routed to human review, gets a
scoped 10-minute credential after approval, and the whole session exports as
verifiable evidence. Proof: [docs/PR_WRITER.md](../PR_WRITER.md).

## Posts (review, then post yourself)

| Platform | File | Notes |
|----------|------|-------|
| Show HN | [posts/hackernews-show-hn.md](posts/hackernews-show-hn.md) | Mechanism-first, includes an honest "what it does NOT do yet" section. Post during US morning. |
| Reddit (r/devops, r/programming) | [posts/reddit.md](posts/reddit.md) | Problem-first, ends with a real ask for feedback. Read each sub's self-promotion rules first. |
| dev.to / blog | [posts/devto-blog.md](posts/devto-blog.md) | Tutorial walkthrough with code blocks. |
| X / Twitter | [posts/x-thread.md](posts/x-thread.md) | 6-tweet thread. Attach the hero GIF to tweet 1. |
| LinkedIn | [posts/linkedin.md](posts/linkedin.md) | Platform/security audience. |
| GitHub Discussions | [posts/github-discussions-announcement.md](posts/github-discussions-announcement.md) | Post in Announcements, pin it. |

## Assets

- [benchmark-card.md](benchmark-card.md) — real numbers, ready to render/screenshot.
- [asset-shot-list.md](asset-shot-list.md) — exactly what GIF + screenshots to capture (manual).

## Suggested order

1. Capture the hero GIF + 3 screenshots ([asset-shot-list.md](asset-shot-list.md)) and upload the social preview.
2. Post the GitHub Discussions announcement and pin it.
3. Post Show HN (the highest-signal, highest-scrutiny channel — make sure assets + proof link are ready first).
4. Cross-post Reddit, dev.to, X, LinkedIn over the following day.
5. Use the same one-line pitch in every one.

## Facts you may cite (and only these)

58,000+ evals/sec (in-process governance benchmark) · 1.1 ms p50 · single-digit-microsecond governance overhead ·
~80% test coverage · Apache-2.0 · Go single binary · v0.8.0. No star/user
counts. Honest about pre-1.0.
