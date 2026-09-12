# Security scenarios, not incident reports

This repository has no verified customer incident report attached to this page. Earlier narratives described hypothetical outcomes as real incidents. Those narratives, timestamps, and loss-prevention claims have been removed.

Use reproducible checks when describing protection:

| Scenario | Available check | Limit |
|---|---|---|
| Destructive routed tool call | [Governed PR proof](../PR_WRITER.md) | Local mock upstream, configured MCP policy |
| Changed arguments or replay after approval | [Approval replay test](../benchmarks/approval-security.md) | Exact test topology and authenticated session |
| Evidence tampering and restart | `scripts/e2e_approval_security.sh` | Requires configured persistence and signing key |
| Direct editor shell, filesystem, or network activity | No automatic interception | Requires external process isolation or explicit routing |

SQL and shell gate libraries have unit tests; they are not installed as operating-system or database sandboxes by the gateway binary. Runtime credential issuance is disabled. Read [runtime support](../runtime-support.md) before describing those libraries as deployed controls.

A future incident report must identify the tested version, routed boundary, configuration, observed failure or block, and sanitized evidence with the reporter's permission. Do not infer zero loss or complete coverage from a policy decision alone.
