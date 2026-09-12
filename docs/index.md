---
title: AegisFlow documentation
description: Install and operate a local-first policy gateway for coding agents and MCP tools.
---

# AegisFlow

AegisFlow is a local-first policy gateway for coding agents. It allows, reviews, or blocks routed MCP actions and writes signed, hash-linked session evidence. Standalone execution gates remain experimental libraries. Runtime upstream credential issuance is disabled. See [runtime support and verification](runtime-support.md).

![AegisFlow governed pull request workflow](assets/hero-pr-writer.gif)

## Run local proof

No provider key or paid service is required.

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow/starter-kit
./install-pr-writer.sh
```

Installer builds binaries, starts local services, loads PR-writer policy, then checks allow, review, and block decisions.

## Pick boundary

| Client or protocol | Start here | Covered behavior |
|---|---|---|
| Claude Code | [Claude Code security](guides/claude-code-security.md) | Messages API and routed MCP calls |
| Cursor | [Cursor policy gateway](guides/cursor-policy-gateway.md) | Routed MCP calls |
| MCP client or server | [MCP gateway security](guides/mcp-gateway-security.md) | Tool discovery, calls, review resume |
| Coding-agent workflow | [Coding-agent governance](guides/coding-agent-governance.md) | Policy decisions, approvals, evidence |

## Know boundary

AegisFlow governs traffic sent through a configured AegisFlow endpoint. It does not sandbox an agent process and cannot stop calls that bypass gateway. Built-in editor file and shell tools need explicit routing through MCP or another supported boundary.

Use [production checklist](production-checklist.md) before exposing services outside local machine. Read [threat model](security/THREAT_MODEL.md) before treating evidence or policy as security control.
