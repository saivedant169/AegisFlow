# AegisFlow Security Questionnaire

**Version:** 1.0
**Date:** 2026-04-06

Pre-answered responses to common enterprise security assessment questions.

---

## 1. Authentication

**Q: What authentication methods does the platform support?**

AegisFlow authenticates all API access via API keys. Each API key is assigned to a tenant and mapped to an RBAC role (admin, operator, or viewer). API keys are passed in the `X-API-Key` header.

For enterprise deployments, AegisFlow supports integration with external identity providers through SSO/OIDC/SAML at the ingress layer. Agent identities are tracked separately from human operator identities.

**Q: How are API keys managed?**

API keys are configured per-tenant in the AegisFlow configuration file. In production, keys should be stored in a secrets manager (HashiCorp Vault, AWS Secrets Manager) and injected at runtime. Keys can be rotated without downtime by adding a new key and removing the old one.

**Q: Does the platform support multi-factor authentication?**

MFA is enforced at the identity provider level for operator access. AegisFlow's admin API accepts authenticated requests from the IdP/SSO layer. API keys used by agents are service credentials and do not use MFA.

---

## 2. Authorization

**Q: What authorization model does the platform use?**

AegisFlow uses a layered authorization model:

1. **RBAC for the control plane:** Three roles (admin, operator, viewer) control access to admin API endpoints. Admins can modify configuration and policies. Operators can approve/deny actions. Viewers have read-only access.

2. **Policy engine for agent actions:** Every agent action is evaluated against a configurable policy engine that produces allow, review, or block decisions. Policies are protocol-aware (MCP, HTTP, shell, SQL, Git) and support allowlists, denylists, capability-based rules, and behavioral detection.

3. **Capability tickets:** Approved actions produce signed capability tickets bound to a specific resource, verb, session, and time window.

**Q: Does the platform support separation of duties?**

Yes. Policy authors cannot approve their own policies. Connector administrators are separate from session operators. Break-glass overrides are audited and require postmortem review.

**Q: Does the platform support least-privilege access?**

Yes. AegisFlow issues task-scoped, short-lived credentials to agents instead of passing through user tokens. Credential scope is bound to the specific action approved. Default policy packs enforce least-privilege by default (strict pack: block destructive, review writes, allow reads).

---

## 3. Encryption

**Q: Is data encrypted in transit?**

Yes. All external communication supports TLS 1.2+ (TLS 1.3 recommended). TLS can be terminated at a load balancer or directly at AegisFlow. All connections to upstream services (GitHub, databases, APIs) use TLS.

**Q: Is data encrypted at rest?**

Evidence chain data stored in PostgreSQL should be encrypted at rest using the database's native encryption (e.g., AWS RDS encryption, GCP Cloud SQL encryption, or dm-crypt for self-managed). Configuration files containing sensitive values should reference environment variables or secrets manager paths, not plaintext secrets.

**Q: What encryption algorithms are used?**

- TLS: AES-256-GCM, ChaCha20-Poly1305
- Evidence chain: SHA-256 hash chain for integrity
- Webhook signing: HMAC-SHA256
- Policy pack signing: Ed25519 or RSA-2048+

---

## 4. Audit Logging

**Q: What actions are logged?**

Every action is logged in the tamper-evident evidence chain:

- All agent action requests (allow, review, block decisions)
- Policy evaluation results with matched rule identification
- Approval and denial records with approver identity
- Credential issuance and revocation records
- Admin API access and configuration changes
- Evidence export and verification events
- Kill switch activations
- Session lifecycle events (start, drift, terminate)

**Q: How is log integrity ensured?**

AegisFlow uses a SHA-256 hash chain for audit records. Each record includes the hash of the previous record, creating a tamper-evident chain. Any modification to a historical record breaks the chain and is detectable via `aegisctl evidence verify`.

**Q: Can logs be exported to external systems?**

Yes. Evidence can be exported in JSON format via the admin API (`/admin/v1/evidence/sessions/{id}/export`). Structured JSON logs (Zap) can be shipped to any log aggregation platform. Prometheus metrics are available at `/metrics`. OpenTelemetry traces can be exported to any OTLP-compatible collector.

**Q: What is the log retention policy?**

Configurable per deployment. Evidence chain retention is controlled by the PostgreSQL storage backend. Recommended minimum retention is 1 year for compliance purposes. Log rotation and archival are managed through standard database backup procedures.

---

## 5. Data Handling

**Q: What data does AegisFlow process?**

AegisFlow processes agent action requests and responses. It does not store model training data, user conversations, or application data. It records:

- Action metadata (tool, target, capability, protocol)
- Policy decisions and justifications
- Approval records
- Credential issuance metadata (not the credentials themselves)
- Evidence chain records

**Q: Does AegisFlow access customer data?**

AegisFlow sees agent action parameters (e.g., SQL queries, shell commands, API payloads) as they pass through the governance layer. PII detection and stripping can be configured to redact sensitive patterns before they reach upstream providers or are recorded in evidence.

**Q: Where is data stored?**

Evidence chain data is stored in the operator's PostgreSQL instance. AegisFlow does not send data to any external service. All processing is local to the deployment.

---

## 6. Incident Response

**Q: Does the platform have an incident response plan?**

Yes. See [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) for the full incident response procedure. Key capabilities:

- Kill switch for immediate session termination
- Policy lock to restrict all non-read operations
- Evidence export for forensic investigation
- Evidence chain verification to confirm audit integrity
- Session timeline reconstruction from evidence records
- Manifest vs. observed diff for scope analysis

**Q: What is the response time for security incidents?**

| Severity | Response Time |
|----------|---------------|
| SEV-1 (active exploitation) | 15 minutes |
| SEV-2 (suspected compromise) | 1 hour |
| SEV-3 (operational issue) | 4 hours |
| SEV-4 (minor issue) | Next business day |

**Q: How are security vulnerabilities reported?**

Report vulnerabilities to the AegisFlow maintainers via the process described in the project's security policy. Provide a description, reproduction steps, and impact assessment.

---

## 7. Compliance

**Q: What compliance frameworks does AegisFlow support?**

AegisFlow's governance model supports compliance with:

- **SOC 2 Type II:** Tamper-evident audit logging, access controls, separation of duties, evidence export for auditors
- **OWASP Top 10 for Agentic Applications:** Full mapping documented in [OWASP_AGENTIC_MAPPING.md](OWASP_AGENTIC_MAPPING.md)
- **NIST AI RMF:** Risk-based governance, human oversight, transparency through evidence
- **ISO 27001:** Access control (A.9), cryptography (A.10), operations security (A.12), logging and monitoring (A.12.4)
- **GDPR:** PII detection and stripping, data minimization through task-scoped access, audit trail for data access
- **HIPAA:** Access controls, audit logging, encryption in transit

AegisFlow is a control layer, not a data processor. Specific compliance certifications depend on the operator's overall deployment and organizational controls.

**Q: Does AegisFlow itself hold any certifications?**

AegisFlow is open-source software. Compliance certifications are the responsibility of the deploying organization. AegisFlow provides the technical controls that support certification efforts.

---

## 8. Vulnerability Management

**Q: How are dependencies managed?**

AegisFlow is written in Go with dependencies managed via Go modules. Dependencies are pinned to specific versions in `go.sum`. The CI pipeline runs `govulncheck` and dependency scanning on every build.

**Q: How are vulnerabilities in AegisFlow itself handled?**

- Security issues are tracked and prioritized
- Critical vulnerabilities are patched within 72 hours
- Security patches are released as point releases
- Release notes include CVE references where applicable

**Q: Is the codebase scanned for vulnerabilities?**

Yes. The CI pipeline includes:
- `go vet` for static analysis
- `govulncheck` for known vulnerability detection
- Dependency scanning for transitive vulnerabilities
- Integration tests that validate security controls

---

## 9. Architecture and Isolation

**Q: How is the platform deployed?**

AegisFlow is a single Go binary with YAML configuration. It can be deployed as:
- A standalone binary
- A Docker container
- A Kubernetes deployment with Helm charts and CRDs
- A multi-cluster federation (control plane + data plane)

**Q: How is tenant isolation enforced?**

Each tenant has a separate API key, rate limits, and policy configuration. Tenants cannot access each other's sessions, evidence, or configuration. In multi-cluster deployments, data plane isolation provides physical separation.

**Q: What is the blast radius of a compromise?**

AegisFlow's credential broker issues task-scoped, short-lived credentials. A compromised agent session can only access the specific resources approved for that task, within the credential TTL. Kill switch terminates the session immediately. The governance layer itself runs with minimal privileges (it does not need or hold broad access to upstream services).

---

## 10. Business Continuity

**Q: What happens if AegisFlow is unavailable?**

In governance mode (default), AegisFlow is fail-closed: if the policy engine cannot evaluate a request, it is blocked. This ensures agents cannot bypass governance during an outage.

A configurable break-glass mode is available for development environments, which allows requests to pass through when the policy engine is unavailable. Break-glass use is always audited.

**Q: What is the platform's availability target?**

AegisFlow supports high availability through:
- Stateless API tier with horizontal scaling
- PostgreSQL with streaming replication for evidence storage
- Redis-backed rate limiting and approval queue for distributed deployments
- Circuit breakers and retry logic for upstream service failures

Specific SLAs depend on the operator's infrastructure and deployment configuration.
