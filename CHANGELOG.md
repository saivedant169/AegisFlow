# Changelog

All notable changes to AegisFlow will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project loosely follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html) while it is pre-1.0.

## [0.5.0] - 2026-04-07

This release is the pivot. AegisFlow used to describe itself as an AI gateway. Now it is an open-source runtime governance layer for tool-using agents. The gateway features are still here, but they sit behind the governance plane as supporting infrastructure.

### Added — Agent execution governance (Phase 6)

- ActionEnvelope core type: every agent action gets normalised into one policy-evaluable object with actor, protocol, tool, target, parameters, requested capability, decision, and an evidence hash
- MCP remote gateway (JSON-RPC 2.0 and SSE transport) on port 8082 so Claude Code, Cursor, and other MCP clients can connect directly
- Tool policy engine with glob matching and three first-class decisions: allow, review, block
- Approval queue for human-in-the-loop review, with submit, approve, deny, and history
- Session evidence chain with SHA-256 hash linking and tamper detection
- Fail-closed governance mode (with a break-glass override for dev/debug)
- aegisctl commands: `test-action`, `pending`, `approve`, `deny`, `verify`, `evidence`, `simulate`, `why`, `diff-policy`, `manifest`, `supply-chain`
- Protocol connectors: shell (with dangerous command detection), SQL (with operation classification), GitHub (with risk tiers), HTTP (reverse proxy with host allowlists)
- 10 end-to-end Phase 6 integration tests

### Added — Task-scoped credentials (Phase 7)

- GitHub App credential broker with real RS256 JWT signing (pure stdlib, no external JWT library)
- AWS STS credential broker with SigV4 signing inlined (no AWS SDK dependency)
- HashiCorp Vault broker for database secrets with lease management
- Static credential broker as a clearly-labelled degraded fallback
- Credential registry with periodic cleanup of expired tokens
- Credential provenance recorded in the evidence chain, so every action is linked to the exact short-lived credential used
- Admin API endpoints for listing active credentials and revoking them

### Added — Evidence, policy packs, benchmarks, and demo (Phase 8)

- Evidence export and session manifests with human-readable reports
- `aegisctl verify` for audit chain and session verification
- `aegisctl evidence` for export and session listing
- Three blessed policy packs: `readonly`, `pr-writer`, `infra-review`
- Governance overhead benchmarks: policy evaluate (~1.2 µs), full allow pipeline (~5.2 µs), review path (~1.3 µs)
- Attack demo pack with 20 scenarios across shell, SQL, GitHub, HTTP, and credential theft
- One-click Docker Compose demo and interactive demo script (9 steps)

### Added — Governed Coding Agent Starter Kit (Phase 9)

- Complete `starter-kit/` directory with everything a team needs to adopt AegisFlow in 15 minutes
- PR-writer focused installer (`install-pr-writer.sh`) with prerequisite checks, sanity tests, and a verified install-to-running time under 10 seconds on a dev machine
- Claude Code and Cursor setup guides with copy-paste configs
- Production deployment templates: Docker Compose, Helm chart, Terraform for AWS ECS Fargate
- Efficacy test pack: 20 attack scenarios plus 2 legitimate operations, pass/fail report output
- Sample evidence bundle and human-readable session report

### Added — Adoption sprint (Phase 10, in progress)

- `docs/PR_WRITER.md` proof page: one concrete scenario walkthrough with real output, real hashes, and real decisions
- Tuned `pr-writer` policy pack: `git status`, `git log`, `git diff`, `pytest`, `go test`, and other everyday commands now pass without interruption; dangerous operations still hard-blocked

### Added — Enterprise grade (all 12 uplift items)

Tier 1 (must-build):
- Typed resource model with hierarchical policy matching and environment awareness
- TaskManifest with intent-to-execution drift detection
- Capability tickets: HMAC-signed one-purpose execution tokens with nonce replay protection
- Policy simulation and explainability (`aegisctl simulate`, `why`, `diff-policy`)
- Safe execution sandboxes for shell, SQL, HTTP, and Git with architectural safety constraints

Tier 2 (hardening):
- Behavioral session policy engine with sequence-based threat detection (exfiltration, privilege escalation, destructive sequences, fan-out, credential abuse)
- Approval integrations: GitHub PR comments and Slack webhooks, with approval timeout auto-deny
- Enterprise identity model: org → team → project → environment hierarchy with separation-of-duties rules
- Signed policy and connector supply chain with trust tiers and strict-mode verification

Tier 3 (operational maturity):
- Resilience: component health monitoring, safe degradation modes, retention policies, backup/restore, circuit breakers
- Threat model (10 threat categories), OWASP Agentic Top 10 control mapping, deployment guide, security questionnaire, saved-incident narratives

### Added — Gateway layer (Phase 5)

- Semantic caching via embedding similarity with a configurable cosine threshold
- Cost optimisation engine that suggests cheaper model alternatives based on real usage
- Request/response transformation: PII stripping from responses, per-tenant system prompt injection, model aliasing
- Load shedding with three priority tiers
- WebSocket endpoint at `/v1/ws` for persistent connections
- GraphQL admin API with 13 queries and 5 mutations alongside the existing REST API
- WASM plugin SDK with tutorial and two example plugins
- 16 new integration tests

### Changed

- README rewritten to lead with the PR-writer workflow and governance positioning, not the gateway one
- Default policy engine mode is now fail-closed (break-glass mode exists for explicit opt-in)
- Gateway features are now described as supporting infrastructure behind the governance plane, not the main story
- Architecture diagram redrawn to show agent → AegisFlow (policy, credentials, evidence) → tools
- Roadmap section reorganised to show Phase 6–10 agent governance work alongside the completed Phase 1–5 gateway work

### Fixed

- MCP gateway now checks approval history before blocking, so an approved action can be retried successfully on the second attempt
- Approval queue is correctly wired into the MCP gateway (was previously passing nil)
- `aegisctl approve` now passes the API key on authenticated endpoints
- Audit endpoint in the demo script now uses the admin API key
- Demo API key role bumped from operator to admin for audit verification access
- MCP gateway upstream URL switched from Docker hostname to `localhost:3000` for local dev
- `tool_policies` rules in the realworld config now use `protocol: "mcp"` instead of `"git"` to match actual MCP transport
- Merge conflict resolution for semantic caching and governance branches

### Security

- Fail-closed is the default behaviour for the policy engine
- Capability tickets prevent replay through nonce and HMAC signing
- Evidence chain integrity is provable via `aegisctl verify`
- Credential provenance means every action is attributable to a specific short-lived credential
- Supply chain signing covers policy packs and connectors (not just WASM plugins)
- Shell sandbox redacts environment variables matching credential patterns

## [0.4.0] - 2026-03-30

### Added — Phase 4: Advanced Governance & Marketplace

#### Enterprise RBAC (4A)
- Three roles per API key: admin, operator, viewer with hierarchy enforcement
- Custom YAML unmarshalling for backward compatibility (plain strings default to operator)
- SoftAuth middleware for admin server (doesn't reject missing keys)
- /admin/v1/whoami endpoint returns current role and tenant
- RBAC middleware with per-route groups (viewer/operator/admin)
- admin_auth.go removed — RBAC replaces it entirely

#### Audit Logging (4B)
- Append-only PostgreSQL table with SHA-256 hash chain
- Buffered channel writer for serialized hash computation
- In-memory store fallback when PostgreSQL is disabled
- /admin/v1/audit endpoint for querying with filters (operator role required)
- /admin/v1/audit/verify endpoint for integrity verification (admin role required)
- Audit Log dashboard page with verify button

#### AI Evaluation Hooks (4C)
- Built-in quality scorer: empty response (0), truncated (-30), too short (-20), latency degradation (-10)
- Webhook evaluator with configurable sampling rate and content truncation
- QualityScore field added to analytics DataPoint
- Wired into gateway handler after every response

#### Plugin Marketplace (4D)
- aegisctl plugin subcommand: search, info, install, list, remove
- JSON registry format hosted on GitHub
- SHA-256 verification on download
- Separate plugins.yaml (never touches main config)
- HTTPS-only download enforcement

#### Multi-Cluster Federation (4E)
- Control plane serves stripped config (API keys removed) to data planes
- Data planes poll config and push metrics/status on configurable interval
- Bearer token authentication between planes
- Federation dashboard page showing data plane health
- Graceful degradation when control plane is down

### Security
- Timing-safe federation token comparison (subtle.ConstantTimeCompare)
- Body size limit on federation status endpoint (1MB)
- Audit endpoint moved from viewer to operator RBAC group (prevents prompt leakage)
- Unicode-safe content truncation in eval webhook
- HTTPS-only enforcement for plugin downloads
- Plugin download size limit (50MB)

## [0.3.0] - 2026-03-27

### Added — Phase 3: Enterprise Capabilities

#### Canary Rollouts (3A)
- Gradual traffic shifting between providers with configurable stages (e.g., 5% → 25% → 50% → 100%)
- Automatic promotion on healthy metrics, auto-rollback on error rate or latency spikes
- Rollout state machine: pending → running → completed/rolled_back, with pause/resume
- PostgreSQL persistence for rollout state (in-memory fallback without DB)
- Admin API: 6 endpoints for rollout lifecycle management (create, list, get, pause, resume, rollback)
- Rollouts dashboard page with live progress, baseline vs canary metrics, action buttons

#### Analytics & Anomaly Detection (3B)
- In-memory time-series collector with 1-minute granularity buckets, 48h rolling window
- Per-dimension tracking (tenant, model, provider, global) with p50/p95/p99 latency
- Static threshold anomaly detection (error rate, latency, request rate, cost)
- Statistical baseline detection (24h moving average + standard deviation)
- Alert manager with auto-resolve after 5 consecutive normal evaluations
- PostgreSQL store for metric aggregates and alert history
- Analytics dashboard page with real-time charts
- Alerts dashboard page with severity badges and acknowledge action

#### Cost Forecasting & Budget Alerts (3C)
- Budget limits at global, per-tenant, and per-model levels
- Three-tier enforcement: alert at 80%, warning header at 90%, block at 100%
- Budget check middleware in request path with per-model enforcement in handler
- Linear cost projection forecasting end-of-period spend
- Budgets dashboard page with spend bars and forecast indicators

#### Multi-Region Routing (3D)
- Region-grouped providers with per-region routing strategy
- Cross-region fallback when all providers in a region are circuit-broken
- Region support in both standard and streaming request paths
- Region field on provider API and live feed dashboard
- Backward compatible — routes without regions work unchanged

#### Kubernetes Operator (3E)
- 5 CRD types: AegisFlowGateway, AegisFlowProvider, AegisFlowRoute, AegisFlowTenant, AegisFlowPolicy
- CRD YAML manifests with printer columns for kubectl
- Operator reconciler: CRDs → aegisflow.yaml ConfigMap
- DeepCopy methods, scheme registration, controller watches
- Status reporting from gateway admin API back to CRD objects
- Operator binary with controller-runtime, RBAC, Deployment manifest, Dockerfile
- Helm chart integration with operator.enabled flag

#### Dashboard
- Gateway Overview now shows Phase 3 operational status (alerts, rollouts, budgets, regions)
- 11 total dashboard pages
- Policy rules display with collapsible "+N more" toggle

### Fixed
- Analytics collector race condition (unlock/relock inside loop)
- Per-model budget enforcement (middleware was passing "*" instead of actual model)
- Streaming requests now use canary rollout and multi-region routing
- Analytics now records all response types (403, 502, 429) not just 200
- Handler dbQueue properly closed on shutdown (goroutine leak fix)
- Nil guard on rollout ActiveRollout lookup

## [0.2.1] - 2026-03-27

### Added
- Cache stats dashboard page with hit/miss counters, hit rate, eviction tracking, and live chart
- Policy violations dashboard page with violation history, per-policy and per-tenant breakdowns
- Cache stats API endpoint (`/admin/v1/cache`)
- Policy violations API endpoint (`/admin/v1/violations`)

### Fixed
- Token rate limiter now reads actual request body size instead of trusting Content-Length header
- Token rate limiter fails closed on error (was failing open)
- Replaced unbounded goroutine DB writes with buffered channel worker (prevents memory leak)

### Changed
- Redis and PostgreSQL ports are now internal-only in Docker Compose (no longer exposed to host)
- Added health check for aegisflow service in Docker Compose
- Added RealIP trusted proxy documentation in gateway setup

## [0.2.0] - 2026-03-26

### Added
- WASM policy plugin support via wazero runtime
- Custom policy filters in any WASM-compatible language (Go, Rust, TinyGo, AssemblyScript)
- Configurable per-plugin timeout and error handling (on_error: block/allow)
- Example WASM plugin with ABI documentation
- Live request feed dashboard page

### Security
- Timing-safe tenant API key comparison (SHA-256 + subtle.ConstantTimeCompare)
- Admin endpoints blocked by default when token is unconfigured
- Removed admin token from URL query params (header-only auth)
- 10MB request body size limit
- Cache keys scoped by tenant ID (cross-tenant data leak fix)
- SSE injection fix via json.Marshal
- Rate limiter fails closed (503) instead of open
- NFKC Unicode normalization for keyword policy filter
- Expanded jailbreak keyword list (3 to 25 patterns)

## [0.1.0] - 2024-03-24

### Added
- Unified AI gateway with OpenAI-compatible API
- Provider adapters: Mock, OpenAI, Anthropic, Ollama
- Intelligent routing with glob-based model matching
- Priority and round-robin routing strategies
- Automatic fallback with circuit breaker
- In-memory rate limiting (sliding window)
- Redis-backed rate limiting (optional)
- Tenant authentication via API keys
- Policy engine with input and output checks
- Keyword blocklist filter
- Regex pattern filter
- PII detection (email, SSN, credit card)
- Usage tracking with per-tenant, per-model aggregation
- Cost estimation per request
- OpenTelemetry tracing (stdout and OTLP exporters)
- Prometheus metrics endpoint
- Structured JSON logging
- Admin API with health, metrics, and usage endpoints
- Docker and Docker Compose support
- GitHub Actions CI/CD
- Comprehensive test suite
