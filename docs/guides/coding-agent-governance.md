---
title: Coding-agent governance at runtime
description: Apply runtime policy, approvals, and signed evidence to coding-agent actions.
---

# Coding-agent governance at runtime

Coding agents can read source, run commands, call APIs, and change remote state. Runtime governance needs decision before tool execution. AegisFlow applies policy at configured protocol boundary.

## Action model

AegisFlow normalizes routed call into `ActionEnvelope` with fields such as:

- actor and task
- protocol and tool
- target and arguments
- requested capability
- session context

Policy returns `allow`, `review`, or `block`.

`allow` sends call upstream. `review` creates pending approval. `block` returns structured error without calling upstream.

## Credentials

Runtime upstream credential issuance is disabled. Authentication keys identify callers; they do not mint provider access. Broker libraries are experimental. See [candidate migration](../releases/corrective-candidate.md).

## Evidence

Each governed session can record policy decisions and upstream results. Records are hash-linked and signed with HMAC key. See [tamper-evident audit guide](tamper-evident-audit.md) for verification and key handling.

## Start with narrow workflow

Use one policy pack and one upstream before broad rollout. PR-writer starter kit covers repository read, test, pull request review, and destructive action block. Run [governed PR proof](../PR_WRITER.md), inspect evidence, then tune target patterns for your repository.
