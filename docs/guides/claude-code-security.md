---
title: Claude Code security with a local policy gateway
description: Route Claude Code model and MCP traffic through AegisFlow for policy checks, approvals, and signed session evidence.
---

# Claude Code security with AegisFlow

AegisFlow can govern two Claude Code paths:

1. Anthropic Messages API requests routed through AegisFlow.
2. MCP tool calls routed through AegisFlow MCP gateway.

These paths have different boundaries. Model routing checks request and response content. MCP routing checks tool names, targets, arguments, capabilities, and session context.

## Route Messages API

Start AegisFlow with an Anthropic provider and point Claude Code at gateway:

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:8080
export ANTHROPIC_API_KEY=<aegisflow-tenant-key>
```

AegisFlow exposes `POST /v1/messages` and `POST /v1/messages/count_tokens`. Input policy runs before provider call. Output policy and evidence handling run before response completes.

The `anthropic-version` header is required for `POST /v1/messages`. If the header is missing, AegisFlow defaults to `2023-06-01` to maintain compatibility with older integrations. If an unsupported version is provided, AegisFlow will reject the request with an Anthropic-compatible JSON error. Valid versions are attached to the request context for auditing.

Tool passthrough on this route is optional and disabled by default:

```yaml
messages_api:
  tool_passthrough: true
```

Test selected provider tool loop before enabling it. Unsupported provider behavior should fail during test, not during an agent task.

## Route MCP tools

Use [Claude Code editor setup](https://github.com/saivedant169/AegisFlow/blob/main/starter-kit/editors/claude-code.md) to register AegisFlow bridge. Gateway checks `tools/list` and `tools/call`, then returns allow, review, or block.

PR-writer pack allows repository reads, reviews pull request creation, and blocks destructive repository operations. Start with local proof:

```bash
cd starter-kit
./install-pr-writer.sh
```

## Boundary limitation

AegisFlow does not intercept Claude Code built-in file or shell operations automatically. Only traffic routed through configured Messages API, MCP gateway, or another supported integration enters policy engine. Keep workspace isolation and host permissions in place.
