# AegisFlow Production Deployment Guide

**Version:** 1.0
**Date:** 2026-04-06

---

## Overview

This guide covers the recommended architecture, configuration, and operational procedures for deploying AegisFlow in production environments. AegisFlow is a single Go binary with YAML configuration, but production deployments require attention to network segmentation, TLS, secrets management, monitoring, and incident response.

---

## Recommended Architecture

```
                        ┌──────────────────────────────────────────┐
                        │           Load Balancer / Ingress        │
                        │         (TLS termination, WAF)           │
                        └──────────┬───────────────┬───────────────┘
                                   │               │
                    ┌──────────────▼──┐   ┌────────▼──────────────┐
                    │  Gateway Port   │   │    Admin Port          │
                    │  (8080)         │   │    (8081)              │
                    │                 │   │                        │
                    │  Agent traffic  │   │  Operator traffic      │
                    │  MCP / HTTP     │   │  Approvals, config,    │
                    │  Chat API       │   │  evidence, metrics     │
                    └────────┬────────┘   └────────┬──────────────┘
                             │                     │
                    ┌────────▼─────────────────────▼──────────────┐
                    │              AegisFlow Process               │
                    │                                              │
                    │  ┌──────────┐ ┌────────────┐ ┌───────────┐  │
                    │  │ Policy   │ │ Credential │ │ Evidence  │  │
                    │  │ Engine   │ │ Broker     │ │ Chain     │  │
                    │  └──────────┘ └────────────┘ └───────────┘  │
                    └────────┬────────────┬───────────┬───────────┘
                             │            │           │
               ┌─────────────▼──┐  ┌──────▼─────┐ ┌──▼──────────┐
               │ Upstream Tools  │  │ Vault /    │ │ PostgreSQL  │
               │ (GitHub, DBs,   │  │ Secrets    │ │ (evidence   │
               │  APIs, shell)   │  │ Manager    │ │  storage)   │
               └─────────────────┘  └────────────┘ └─────────────┘
```

### Deployment options

| Option | Best for | Notes |
|--------|----------|-------|
| Single binary | Small teams, dev/staging | `make build && ./aegisflow -config config.yaml` |
| Docker Compose | Medium teams, single-host | `deployments/docker-compose.yaml` |
| Kubernetes + Helm | Enterprise, multi-cluster | `deployments/helm/` with CRDs and operator |
| Multi-cluster federation | Large enterprises | Control plane + data plane separation |

---

## Network Segmentation

AegisFlow exposes three distinct network surfaces. Each should be isolated.

### Gateway Port (default: 8080)

- **Purpose:** Agent-facing traffic (chat completions, MCP tool calls, WebSocket)
- **Access:** Agents and MCP clients only
- **Network policy:** Allow inbound from agent hosts/VPC; deny all other inbound
- **Rate limiting:** Enforced per-tenant at this port

### Admin Port (default: 8081)

- **Purpose:** Operator traffic (approvals, evidence, config, metrics, GraphQL)
- **Access:** Operators and monitoring infrastructure only
- **Network policy:** Allow inbound only from operator workstations and monitoring VPC; deny all agent traffic
- **Authentication:** API key with RBAC (admin, operator, viewer)

### Metrics Endpoint (/metrics on admin port)

- **Purpose:** Prometheus scraping
- **Access:** Monitoring infrastructure only
- **Network policy:** Allow inbound only from Prometheus scraper IPs

### Recommended network layout

```
┌─────────────────────┐     ┌─────────────────────┐     ┌─────────────────────┐
│   Agent Network      │     │   Operator Network   │     │  Monitoring Network  │
│                      │     │                      │     │                      │
│  Agents, MCP clients │     │  Operator consoles   │     │  Prometheus, Grafana │
│                      │     │  aegisctl CLI        │     │  Alertmanager        │
└──────────┬───────────┘     └──────────┬───────────┘     └──────────┬───────────┘
           │ :8080                      │ :8081                      │ :8081/metrics
           ▼                            ▼                            ▼
┌──────────────────────────────────────────────────────────────────────────────────┐
│                              AegisFlow                                           │
└──────────────────────────────────────────────────────────────────────────────────┘
```

---

## TLS Configuration

### Requirements

- TLS 1.2 minimum; TLS 1.3 recommended
- Strong cipher suites only (AES-256-GCM, ChaCha20-Poly1305)
- Certificate rotation on a regular schedule (90 days recommended)

### Option 1: TLS termination at load balancer (recommended)

Use your existing load balancer or ingress controller (AWS ALB, Nginx, Envoy, Istio) to terminate TLS. AegisFlow communicates over plaintext within the trusted network.

```yaml
server:
  port: 8080
  admin_port: 8081
  # No TLS config needed when terminating at LB
```

### Option 2: TLS termination at AegisFlow

For deployments without a load balancer, configure TLS directly:

```yaml
server:
  port: 8080
  admin_port: 8081
  tls:
    enabled: true
    cert_file: "/etc/aegisflow/tls/server.crt"
    key_file: "/etc/aegisflow/tls/server.key"
    min_version: "1.2"
```

### Upstream TLS

All connections to upstream services (GitHub API, databases, HTTP endpoints) should use TLS. Configure provider base URLs with `https://` and verify certificates.

---

## Secrets Management

### Principles

1. Never store secrets in configuration files or environment variables visible to agents
2. Use a dedicated secrets manager (HashiCorp Vault, AWS Secrets Manager, GCP Secret Manager)
3. AegisFlow's credential broker issues task-scoped, short-lived credentials to agents
4. Rotate all long-lived secrets on a regular schedule

### Secret types and storage

| Secret | Storage | Rotation |
|--------|---------|----------|
| Provider API keys (OpenAI, Anthropic) | Vault / cloud secrets manager | 90 days |
| AegisFlow admin API keys | Vault / K8s Secret | 90 days |
| Database credentials (evidence store) | Vault dynamic secrets | Per-connection |
| GitHub App private key | Vault | Annual |
| TLS certificates | cert-manager / Vault PKI | 90 days |
| HMAC signing keys (webhooks) | Vault | 90 days |
| Policy pack signing keys | Vault / HSM | Annual |

### Configuration

Use environment variable references in configuration, not literal values:

```yaml
providers:
  - name: "openai"
    api_key_env: "OPENAI_API_KEY"    # Reads from environment at startup
```

For Kubernetes, use `ExternalSecret` or Vault Agent sidecar to inject secrets as environment variables.

---

## Monitoring Integration

### Prometheus metrics

AegisFlow exposes metrics at `/metrics` on the admin port:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'aegisflow'
    scrape_interval: 15s
    static_configs:
      - targets: ['aegisflow-admin:8081']
    metrics_path: '/metrics'
```

### Key metrics to alert on

| Metric | Alert threshold | Severity |
|--------|----------------|----------|
| `aegisflow_policy_decisions_total{decision="block"}` | Spike > 3x baseline | Warning |
| `aegisflow_approval_queue_depth` | > 50 pending | Warning |
| `aegisflow_evidence_chain_errors` | > 0 | Critical |
| `aegisflow_credential_issuance_failures` | > 0 | Critical |
| `aegisflow_request_latency_p99` | > 100ms | Warning |
| `aegisflow_session_drift_events` | Any | Warning |
| `aegisflow_kill_switch_activations` | Any | Critical |

### OpenTelemetry tracing

Configure the OTLP exporter to send traces to your collector:

```yaml
telemetry:
  otlp_endpoint: "otel-collector:4317"
  service_name: "aegisflow"
```

### Log aggregation

AegisFlow outputs structured JSON logs (Zap). Ship to your log aggregation platform (Elasticsearch, Loki, Splunk, Datadog):

```bash
# Example: ship logs via fluent-bit sidecar
aegisflow -config config.yaml 2>&1 | fluent-bit -i stdin -o elasticsearch ...
```

---

## Backup Strategy

### What to back up

| Data | Method | Frequency | Retention |
|------|--------|-----------|-----------|
| Evidence chain (PostgreSQL) | pg_dump / WAL archiving | Continuous | Per compliance policy (min 1 year) |
| Configuration files | Git version control | On every change | Indefinite |
| Policy packs | Git version control | On every change | Indefinite |
| Signing keys | Vault snapshot | Daily | Match key lifecycle |
| Approval records | Included in evidence chain | Continuous | Per compliance policy |

### PostgreSQL backup

```bash
# Automated daily backup
pg_dump -Fc aegisflow_evidence > /backups/evidence_$(date +%Y%m%d).dump

# Point-in-time recovery via WAL archiving
archive_mode = on
archive_command = 'cp %p /wal_archive/%f'
```

### Evidence verification after restore

After restoring from backup, always verify the evidence chain:

```bash
aegisctl evidence verify --all-sessions
```

---

## High Availability

### Stateless API tier

AegisFlow's gateway and admin API are stateless. Run multiple replicas behind a load balancer:

```yaml
# Kubernetes deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aegisflow
spec:
  replicas: 3
  ...
```

### Durable approval queue

For HA deployments, use PostgreSQL or Redis as the approval queue backend instead of in-memory:

```yaml
approval_queue:
  backend: "postgres"   # or "redis"
```

### Evidence chain

Use PostgreSQL with streaming replication for the evidence store:

```yaml
storage:
  type: "postgres"
  dsn_env: "AEGISFLOW_DB_DSN"
```

---

## Incident Response Procedures

### Severity Levels

| Level | Definition | Response Time | Example |
|-------|-----------|---------------|---------|
| SEV-1 | Active exploitation or governance bypass | 15 minutes | Evidence chain compromised, policy engine crash |
| SEV-2 | Suspected compromise or anomalous behavior | 1 hour | Unusual spike in blocked actions, drift events |
| SEV-3 | Operational issue affecting governance | 4 hours | Approval queue backlog, credential broker timeout |
| SEV-4 | Minor issue, no governance impact | Next business day | Metric collection gap, log shipping delay |

### Incident Response Steps

**1. Detection and triage**
- Monitor alerts from Prometheus, SIEM, and AegisFlow's anomaly detection
- Check `/admin/v1/alerts` for recent alerts
- Determine severity level

**2. Containment**
- **Kill switch:** Terminate suspicious sessions immediately via `aegisctl kill-session <id>`
- **Policy lock:** Switch to strict policy pack to block all non-read operations
- **Network isolation:** Block agent network access to the gateway port if needed
- **Credential revocation:** Revoke all active credentials via the credential broker

**3. Investigation**
- Export evidence for affected sessions: `aegisctl evidence export --session <id>`
- Verify evidence chain integrity: `aegisctl evidence verify --session <id>`
- Review the session timeline: action sequence, policy decisions, approval records
- Compare actual behavior against TaskManifest (declared vs. observed)
- Check audit log: `/admin/v1/audit`

**4. Eradication**
- Identify the root cause (compromised agent, malicious input, policy gap)
- Update policies to close the gap
- Test updated policies with `aegisctl simulate`
- Rotate any potentially compromised credentials

**5. Recovery**
- Re-enable normal operations with updated policies
- Monitor closely for recurrence
- Verify evidence chain remains intact

**6. Post-incident**
- Document the incident timeline, root cause, and remediation
- Update the threat model if a new threat category is identified
- Review and update this deployment guide as needed
- Share lessons learned with the team

### Emergency Contacts

Maintain a runbook with:
- On-call rotation for AegisFlow operators
- Escalation path to security team
- Contact information for upstream service owners (GitHub, cloud providers)
- Break-glass procedures and who is authorized to use them
