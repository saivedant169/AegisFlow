# Changelog

All notable changes to AegisFlow will be documented in this file.

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
