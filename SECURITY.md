# Security Policy

AegisFlow is a runtime governance layer for tool-using agents. Security reports are something I take seriously, because the whole point of the project is to stop bad things from happening.

If you find a vulnerability, I would rather hear about it quietly first than read about it on Hacker News.

## Supported versions

At this stage the project is pre-1.0 and I only patch the latest `main`. Once there is a stable release line, this section will list supported versions explicitly.

| Version | Supported |
| ------- | --------- |
| `main`  | Yes       |
| 0.5.x   | Yes (current release line) |
| 0.4.x   | No        |
| Older   | No        |

## Reporting a vulnerability

Please do **not** open a public GitHub issue for security reports. Use one of these instead:

1. **Private security advisory on GitHub**: go to the [Security tab](https://github.com/saivedant169/AegisFlow/security/advisories/new) and open a new advisory. This is the preferred channel because it keeps everything tracked in one place.
2. **Email**: `saivedant169@gmail.com` with the subject line `[aegisflow security]`. PGP is not set up yet.

When you report, it helps a lot if you can include:

- A short description of the issue
- Steps to reproduce (a curl command, a config snippet, or a small Go test is ideal)
- The version or commit hash you tested against
- What you think the impact is
- Whether you want credit in the fix (and how you would like to be credited)

I will acknowledge receipt within 72 hours. Triage and a fix plan usually follow within 7 days for critical issues, longer for lower severity. If I am slow, feel free to nudge me.

## What is in scope

These are the parts of the project where security reports are most useful:

- **Policy engine and tool policy matching** (`internal/policy`, `internal/toolpolicy`): any way to bypass an allow/review/block decision, or to make a rule match something it should not
- **Evidence chain** (`internal/evidence`, `internal/audit`): any way to tamper with the hash chain without detection, or to forge an evidence record
- **Capability tickets** (`internal/capability`): signature forgery, replay bypass, nonce prediction
- **Credential brokers** (`internal/credential`): token leakage, scope escalation, privilege boundary crossing, provenance omission
- **Approval queue** (`internal/approval`): queue bypass, double-approval, expired item revival, cross-session leakage
- **MCP gateway** (`internal/mcpgw`): SSE hijacking, session confusion, upstream request smuggling
- **Protocol connectors** (`internal/shellgate`, `internal/sqlgate`, `internal/githubgate`, `internal/httpgate`): sandbox escape, dangerous-command detection bypass, SQL classification misses
- **Sandboxes** (`internal/sandbox`): any way to escape the shell, SQL, HTTP, or Git sandbox constraints
- **Admin API** (`internal/admin`): auth bypass, RBAC bypass, path traversal, injection
- **Federation** (`internal/federation`): cross-plane trust issues, token comparison timing, config leakage
- **Supply chain signing** (`internal/supply`): signature verification bypass, trust tier confusion
- **Gateway middleware** (`internal/middleware`): authentication bypass, rate limit bypass, CORS misconfigurations

## What is out of scope

These are not security vulnerabilities and should be filed as regular issues or PRs:

- Bugs in the mock provider (`internal/provider/mock.go`) or the demo mock MCP server (`scripts/mock-mcp-server.js`). These are for testing and demos, not production.
- Content quality issues in the policy packs themselves (for example, a pack being too restrictive). Open an issue.
- DoS from unrealistic load. If you can show a realistic DoS path with the single Go binary under normal limits, that is in scope. If you are running 1M RPS from a single client without rate limits, that is not.
- Third-party service issues (GitHub API, AWS STS, Vault). Report those to the upstream vendor.
- Issues that require the attacker to already have admin-role API keys or control over the host. Those are operator mistakes, not gateway vulnerabilities.
- The `AegisFlow_Enterprise_Grade_Uplift_Plan_2026-04-06.md` and similar planning documents (these are gitignored and not shipped).

## Safe harbour

If you are reporting in good faith, you will not be pursued for testing against your own AegisFlow instance. Please do not:

- Test against instances you do not own or have explicit permission to test
- Access or modify data that does not belong to you
- Perform DoS testing against public instances
- Use findings to pivot into other systems

## Bug bounty

There is no formal bounty program right now. If a report saves real trouble, I will absolutely acknowledge you in the release notes, and if the project starts making money I will come back and thank people properly.

## Disclosure timeline

My default timeline for coordinated disclosure is:

- Day 0: report received, I acknowledge
- Day 7: triage complete, severity agreed, fix plan in place
- Day 30: fix released for critical issues (longer for lower severity, but I will tell you if it slips)
- Day 90 or at fix release, whichever is later: public advisory published

If you have a hard deadline (for example, you are writing a conference talk), tell me up front and I will work to it.

## A short note on what AegisFlow actually protects against

AegisFlow sits between coding agents and the tools those agents can touch. It is designed to stop an agent from doing something destructive, out of scope, or unattributable, even when the agent itself is compromised or manipulated.

That means the threats that matter most are the ones in the trust boundary between the agent and the tool. Those are the reports I really want to see.

See `docs/security/THREAT_MODEL.md` for the full threat categories I think about.

Thanks for reading this far. Security reports help more than almost anything else.
