# Phase 3: Enterprise Capabilities — Design Specification

**Date:** 2026-03-27
**Status:** Approved
**Scope:** Five sub-projects delivering enterprise-grade capabilities to AegisFlow

## Overview

Phase 3 transforms AegisFlow from a single-node AI gateway into an enterprise-ready platform with gradual model rollouts, intelligent anomaly detection, cost governance, geo-aware routing, and Kubernetes-native configuration. Each sub-project is independent and builds on the previous in this order:

| Order | Sub-project | Depends on |
|-------|-------------|------------|
| 3A | A/B Testing & Canary Deployments | Existing router |
| 3B | Advanced Analytics & Anomaly Detection | Existing usage tracking |
| 3C | Cost Forecasting & Budget Alerts | 3B (analytics data) |
| 3D | Multi-Region Routing | Existing router + config |
| 3E | Kubernetes Operator with CRDs | All config structures finalized |

---

## 3A: A/B Testing & Canary Deployments for Models

### Goal

Allow operators to gradually shift traffic from one provider to another for a given route, with automatic health-based promotion or rollback.

### Architecture

A new `internal/rollout/` package manages gradual traffic shifting between providers. The rollout engine runs as a background goroutine that evaluates health metrics at each observation window, auto-promotes to the next stage or auto-rolls back. State is persisted in PostgreSQL (falls back to in-memory without it).

The existing `router.Router` gets a thin integration layer: before selecting a provider via the current strategy, it checks if an active rollout exists for the matched route and uses weighted random selection between the baseline and canary provider based on the current rollout percentage.

### Components

| Component | Path | Purpose |
|-----------|------|---------|
| `manager.go` | `internal/rollout/` | Rollout lifecycle: create, promote, pause, resume, rollback, complete |
| `evaluator.go` | `internal/rollout/` | Health evaluation: error rate + p95 latency per provider, decides promote/rollback |
| `store.go` | `internal/rollout/` | PostgreSQL persistence for rollout state |
| `memory_store.go` | `internal/rollout/` | In-memory fallback when PostgreSQL is disabled |
| Router integration | `internal/router/router.go` | Check active rollout before provider selection, apply weighted routing |
| Admin API endpoints | `internal/admin/admin.go` | CRUD endpoints for rollout management |
| Dashboard page | `internal/admin/dashboard.html` | Rollouts page with live progress, actions |
| Config fields | `internal/config/config.go` | Canary config on `RouteConfig` |

### Config

```yaml
routes:
  - match:
      model: "gpt-4o"
    providers: ["openai"]
    strategy: "priority"
    canary:
      target_provider: "azure_openai"
      stages: [5, 25, 50, 100]
      observation_window: 5m
      error_threshold: 5.0
      latency_p95_threshold: 3000
```

#### Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `target_provider` | string | yes | The canary provider to gradually shift traffic to |
| `stages` | []int | yes | Percentage steps, each must be higher than previous, last must be 100 |
| `observation_window` | duration | yes | How long to observe at each stage before promoting |
| `error_threshold` | float64 | yes | Max error rate percentage before rollback |
| `latency_p95_threshold` | int64 | yes | Max p95 latency in milliseconds before rollback |

### Rollout State Machine

```
PENDING ─────► RUNNING ─────► [auto-promote through stages] ─────► COMPLETED
                 │                          │
                 ▼                          ▼
               PAUSED                  ROLLED_BACK
                 │
                 ▼
               RUNNING (resume)
```

States:
- `pending` — created but not yet started (waiting for first observation window)
- `running` — actively routing canary traffic, evaluating health
- `paused` — traffic split frozen at current stage, no evaluation
- `rolled_back` — canary traffic returned to 0%, baseline restored
- `completed` — all stages passed, canary is now the primary provider

### Rollout Table Schema

```sql
CREATE TABLE IF NOT EXISTS rollouts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    route_model VARCHAR(255) NOT NULL,
    baseline_providers TEXT NOT NULL,
    canary_provider VARCHAR(255) NOT NULL,
    stages TEXT NOT NULL,
    current_stage INT NOT NULL DEFAULT 0,
    current_percentage INT NOT NULL DEFAULT 0,
    state VARCHAR(20) NOT NULL DEFAULT 'pending',
    observation_window BIGINT NOT NULL,
    error_threshold DOUBLE PRECISION NOT NULL,
    latency_p95_threshold BIGINT NOT NULL,
    stage_started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    rollback_reason TEXT
);
```

### Admin API

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/admin/v1/rollouts` | GET | List all rollouts (active + recent completed) |
| `/admin/v1/rollouts` | POST | Start a new rollout |
| `/admin/v1/rollouts/{id}` | GET | Get rollout details with current metrics |
| `/admin/v1/rollouts/{id}/pause` | POST | Pause an active rollout |
| `/admin/v1/rollouts/{id}/resume` | POST | Resume a paused rollout |
| `/admin/v1/rollouts/{id}/rollback` | POST | Force immediate rollback |

#### POST /admin/v1/rollouts Request Body

```json
{
  "route_model": "gpt-4o",
  "canary_provider": "azure_openai",
  "stages": [5, 25, 50, 100],
  "observation_window": "5m",
  "error_threshold": 5.0,
  "latency_p95_threshold": 3000
}
```

#### GET /admin/v1/rollouts/{id} Response

```json
{
  "id": "uuid",
  "route_model": "gpt-4o",
  "canary_provider": "azure_openai",
  "state": "running",
  "current_stage": 1,
  "current_percentage": 25,
  "stages": [5, 25, 50, 100],
  "observation_window": "5m",
  "metrics": {
    "baseline": {"error_rate": 1.2, "p95_latency_ms": 450, "requests": 892},
    "canary": {"error_rate": 0.8, "p95_latency_ms": 520, "requests": 298}
  },
  "stage_started_at": "2026-03-27T10:00:00Z",
  "time_remaining": "2m30s",
  "created_at": "2026-03-27T09:45:00Z"
}
```

### Health Evaluation Logic

The evaluator runs every `observation_window` for each active rollout:

1. Query recent requests for the canary provider from `RequestLog` (last `observation_window` duration)
2. Calculate error rate: `(5xx responses) / (total requests) * 100`
3. Calculate p95 latency: sort latency_ms values, take 95th percentile
4. If error rate > `error_threshold` OR p95 > `latency_p95_threshold`: set state to `rolled_back`, set `current_percentage` to 0, record `rollback_reason`
5. If both within bounds AND observation window elapsed: advance `current_stage`, set `current_percentage` to `stages[current_stage]`, reset `stage_started_at`
6. If at final stage (100%) and healthy: set state to `completed`, set `completed_at`
7. Minimum 10 requests required in observation window before making a decision (avoid rollback on insufficient data)

### Router Integration

In `router.go`, the `Route()` method:

1. Match route as normal
2. Call `rolloutManager.ActiveRollout(model)` — returns `nil` or active rollout
3. If active rollout exists and state is `running`:
   - Generate `rand.Intn(100)`
   - If < `current_percentage`: route to canary provider
   - Otherwise: route to baseline providers using normal strategy
4. If no active rollout: existing routing logic unchanged
5. After routing: tag the request in `RequestLog` with `provider` field for evaluator to query

### Dashboard — Rollouts Page

Nav item: "Rollouts" under Monitoring section.

Stat cards:
- Active Rollouts (count)
- Completed This Week (count)
- Rolled Back This Week (count)

Active rollout cards (one per active rollout):
- Route model name
- Canary provider name
- Progress bar showing current stage position within stages array
- Current percentage (large number)
- Time remaining in current observation window (countdown)
- Baseline vs canary metrics side by side: error rate, p95 latency, request count
- Action buttons: Pause, Rollback

History table:
- Recent completed/rolled-back rollouts with route, canary, outcome, duration, reason

### Error Handling

| Scenario | Behavior |
|----------|----------|
| PostgreSQL down during rollout | Continue with last known state in memory, log warning |
| Canary provider unreachable | 100% error rate triggers immediate rollback |
| Gateway restart during rollout | Reload state from PostgreSQL, resume observation timer from `stage_started_at` |
| Multiple gateways | All read same state from PostgreSQL, routing is probabilistic per instance |
| Fewer than 10 canary requests in observation window | Skip evaluation, extend observation by another window |
| Rollout started for model with no matching route | Return 400 error on API call |

### Testing

- Unit tests for `evaluator.go`: healthy promote, unhealthy rollback, insufficient data skip, boundary conditions
- Unit tests for `manager.go`: state transitions (pending→running→completed, running→paused→running, running→rolled_back)
- Unit tests for weighted routing: verify traffic split percentages are correct over N requests (within statistical tolerance)
- Integration test: create rollout via API, simulate healthy requests, verify auto-promotion through all stages to completed
- Integration test: create rollout, simulate errors exceeding threshold, verify auto-rollback
- Integration test: pause during rollout, verify traffic split freezes, resume and verify it continues

---

## 3B: Advanced Analytics & Anomaly Detection

### Goal

Provide real-time and historical analytics for all gateway traffic, with automatic anomaly detection that catches traffic spikes, performance degradation, and cost anomalies.

### Architecture

A new `internal/analytics/` package with two layers:

1. **Time-series collector**: in-memory ring buffer of 1-minute granularity buckets, 48h rolling window. Fed by the gateway handler after each request.
2. **Anomaly detector**: evaluates every minute against static thresholds (operator-configured hard limits) and statistical baselines (rolling 24h moving average + standard deviation). Fires alerts via webhook + dashboard.

Hourly and daily aggregates are flushed to PostgreSQL for historical analytics and dashboard charts.

### Components

| Component | Path | Purpose |
|-----------|------|---------|
| `collector.go` | `internal/analytics/` | In-memory time-series ring buffer, 1-min buckets, per-tenant/model/provider |
| `aggregator.go` | `internal/analytics/` | Flush hourly/daily aggregates to PostgreSQL |
| `detector.go` | `internal/analytics/` | Static threshold + statistical baseline anomaly detection |
| `alerts.go` | `internal/analytics/` | Alert lifecycle: create, acknowledge, resolve. Fires webhooks |
| `store.go` | `internal/analytics/` | PostgreSQL tables for aggregates + alert history |
| Admin API | `internal/admin/` | `/admin/v1/analytics`, `/admin/v1/alerts` |
| Dashboard | `internal/admin/dashboard.html` | Analytics page + Alerts panel |

### Metrics Tracked

Per 1-minute bucket, per dimension (tenant_id, model, provider):

| Metric | Type | Description |
|--------|------|-------------|
| `request_count` | counter | Total requests in this bucket |
| `error_count` | counter | 5xx responses |
| `error_rate` | gauge | `error_count / request_count * 100` |
| `latency_p50` | gauge | Median latency in ms |
| `latency_p95` | gauge | 95th percentile latency in ms |
| `latency_p99` | gauge | 99th percentile latency in ms |
| `token_count` | counter | Total tokens (prompt + completion) |
| `estimated_cost` | counter | Estimated USD cost |

### In-Memory Ring Buffer

```go
type MetricBucket struct {
    Timestamp    time.Time
    Requests     int64
    Errors       int64
    Latencies    []int64  // raw values for percentile calc
    Tokens       int64
    EstimatedCost float64
}

type TimeSeries struct {
    mu      sync.RWMutex
    buckets []MetricBucket  // 2880 buckets = 48h at 1-min granularity
    pos     int
}
```

Dimensions: `map[string]*TimeSeries` keyed by `"tenant:{id}"`, `"model:{name}"`, `"provider:{name}"`.

Memory estimate: ~2880 buckets x ~50 dimensions x ~200 bytes/bucket = ~28MB. Acceptable.

### Anomaly Detection Config

```yaml
analytics:
  enabled: true
  retention_hours: 48
  flush_interval: 1h
  anomaly_detection:
    enabled: true
    evaluation_interval: 1m
    static:
      error_rate_max: 20
      p95_latency_max: 5000
      requests_per_minute_max: 10000
      cost_per_minute_max: 50.0
    baseline:
      window: 24h
      stddev_threshold: 3
```

### Static Thresholds

Hard limits configured by operator. Any metric exceeding its threshold fires a `critical` alert immediately.

| Metric | Config field | Default | Alert when |
|--------|-------------|---------|------------|
| Error rate | `error_rate_max` | 20% | `error_rate > error_rate_max` |
| p95 latency | `p95_latency_max` | 5000ms | `p95 > p95_latency_max` |
| Request rate | `requests_per_minute_max` | 10000 | `requests > requests_per_minute_max` |
| Cost rate | `cost_per_minute_max` | $50 | `cost > cost_per_minute_max` |

### Statistical Baseline

Rolling 24h moving average and standard deviation per metric per dimension. Alert when the current 5-minute average deviates beyond N standard deviations.

Algorithm:
1. For each metric, compute `mean` and `stddev` over the last 24h of 1-minute buckets (1440 values)
2. Compute `current_avg` over the last 5 minutes (5 values)
3. If `|current_avg - mean| > stddev * stddev_threshold`: fire `warning` alert
4. Skip detection if fewer than 60 baseline buckets (less than 1h of data — insufficient baseline)

### Alert Severity

| Severity | Trigger | Example |
|----------|---------|---------|
| `critical` | Static threshold exceeded | Error rate 25% (max 20%) |
| `warning` | Statistical anomaly (>N stddev) | Request rate 3.5x normal |
| `info` | Notable event, within bounds | New model first seen, provider recovered |

### Alert Structure

```go
type Alert struct {
    ID        string    `json:"id"`
    Severity  string    `json:"severity"`
    Type      string    `json:"type"`
    Dimension string    `json:"dimension"`
    Metric    string    `json:"metric"`
    Value     float64   `json:"value"`
    Threshold float64   `json:"threshold"`
    Message   string    `json:"message"`
    State     string    `json:"state"`
    CreatedAt time.Time `json:"created_at"`
    ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
```

Alert states: `active` → `resolved` (auto-resolves when metric returns to normal for 5 consecutive minutes).

### Alert Table Schema

```sql
CREATE TABLE IF NOT EXISTS alerts (
    id VARCHAR(36) PRIMARY KEY,
    severity VARCHAR(10) NOT NULL,
    type VARCHAR(50) NOT NULL,
    dimension VARCHAR(255) NOT NULL,
    metric VARCHAR(50) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    threshold DOUBLE PRECISION NOT NULL,
    message TEXT NOT NULL,
    state VARCHAR(10) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_alerts_state ON alerts(state);
CREATE INDEX IF NOT EXISTS idx_alerts_created ON alerts(created_at);
```

### Aggregate Table Schema

```sql
CREATE TABLE IF NOT EXISTS metric_aggregates (
    id BIGSERIAL PRIMARY KEY,
    dimension VARCHAR(255) NOT NULL,
    period VARCHAR(10) NOT NULL,
    bucket_start TIMESTAMPTZ NOT NULL,
    request_count BIGINT NOT NULL DEFAULT 0,
    error_count BIGINT NOT NULL DEFAULT 0,
    p50_latency BIGINT NOT NULL DEFAULT 0,
    p95_latency BIGINT NOT NULL DEFAULT 0,
    p99_latency BIGINT NOT NULL DEFAULT 0,
    token_count BIGINT NOT NULL DEFAULT 0,
    estimated_cost DOUBLE PRECISION NOT NULL DEFAULT 0,
    UNIQUE(dimension, period, bucket_start)
);

CREATE INDEX IF NOT EXISTS idx_aggregates_dimension ON metric_aggregates(dimension, period, bucket_start);
```

### Admin API

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/admin/v1/analytics` | GET | Query aggregated metrics. Params: `dimension`, `period` (hourly/daily), `from`, `to` |
| `/admin/v1/analytics/realtime` | GET | Current 5-min metrics for all dimensions |
| `/admin/v1/alerts` | GET | List alerts. Params: `state` (active/resolved), `severity`, `limit` |
| `/admin/v1/alerts/{id}/acknowledge` | POST | Acknowledge an alert (doesn't resolve, just marks seen) |

### Dashboard — Analytics Page

Nav item: "Analytics" under Monitoring section.

- Time-series charts: request rate, error rate, latency (p50/p95/p99), cost — filterable by tenant/model/provider
- Timeframe selector: 1h, 6h, 24h, 7d
- Real-time metrics cards: current request rate, error rate, avg latency, cost/hour

### Dashboard — Alerts Panel

Shown as a notification badge on the nav + dedicated section on Analytics page.

- Active alerts list with severity icon, dimension, metric, value, time
- Resolved alerts (last 24h) below active
- Alert count badge on sidebar nav item

### Gateway Integration

In `handler.go`, after each request completes:

```go
if h.analytics != nil {
    h.analytics.Record(analytics.DataPoint{
        TenantID:  tenantID,
        Model:     req.Model,
        Provider:  providerName,
        StatusCode: statusCode,
        LatencyMs:  latencyMs,
        Tokens:    totalTokens,
        Cost:      estimatedCost,
    })
}
```

The collector handles bucketing internally. Zero allocation on the hot path (pre-allocated buckets).

### Testing

- Unit tests for collector: record data points, verify bucket aggregation, verify ring buffer wraparound
- Unit tests for detector: static threshold fire/clear, statistical baseline fire/clear, insufficient data skip
- Unit tests for alert lifecycle: create, auto-resolve after metric normalizes
- Integration test: record data points that exceed threshold, verify alert created and webhook fired
- Integration test: record normal data, then spike, verify statistical anomaly detected

---

## 3C: Cost Forecasting & Budget Alerts

### Goal

Allow operators to set spending limits at global, per-tenant, and per-model levels with automatic alerts and enforcement.

### Architecture

A new `internal/budget/` package. Reads cost data from the analytics collector (3B), checks against configured budgets every minute. Tracks spend accumulation per budget period. Forecasts using linear projection over last 24h.

### Components

| Component | Path | Purpose |
|-----------|------|---------|
| `budget.go` | `internal/budget/` | Budget evaluation: check spend vs limits, trigger actions |
| `forecast.go` | `internal/budget/` | Linear projection of spend to end of period |
| `store.go` | `internal/budget/` | PostgreSQL persistence for spend tracking |
| `budget.go` | `internal/middleware/` | Request-path budget check middleware |
| Admin API | `internal/admin/` | `/admin/v1/budgets` |
| Dashboard | `internal/admin/dashboard.html` | Budgets page |

### Config

```yaml
budgets:
  enabled: true
  global:
    monthly: 5000
    alert_at: 80
    warn_at: 90
  tenants:
    premium:
      monthly: 500
      daily: 50
      alert_at: 80
      warn_at: 90
      models:
        gpt-4o:
          monthly: 200
          alert_at: 75
          warn_at: 90
    default:
      monthly: 100
      alert_at: 80
      warn_at: 90
```

#### Budget Config Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `monthly` | float64 | no | 0 (unlimited) | Monthly budget in USD |
| `daily` | float64 | no | 0 (unlimited) | Daily budget in USD |
| `alert_at` | int | no | 80 | Percentage to fire alert webhook |
| `warn_at` | int | no | 90 | Percentage to add warning header |

### Budget Hierarchy

Evaluation order (all checked, most restrictive wins):

1. Model-level budget (tenant + model)
2. Tenant-level budget
3. Global budget

If any level is at 100%, request is blocked regardless of other levels.

### Behavior at Thresholds

| Spend % | Action | Details |
|---------|--------|---------|
| < alert_at | Normal | No action |
| >= alert_at (80%) | Alert | Webhook with `budget_alert` event, dashboard warning |
| >= warn_at (90%) | Warn | `X-AegisFlow-Budget-Warning: 92%` response header added |
| >= 100% | Block | 429 response: `{"error":"budget_exceeded","message":"monthly budget of $500.00 exhausted (current: $502.30)","budget_type":"tenant","tenant_id":"premium"}` |

### Budget Spend Table Schema

```sql
CREATE TABLE IF NOT EXISTS budget_spend (
    id BIGSERIAL PRIMARY KEY,
    scope VARCHAR(50) NOT NULL,
    scope_id VARCHAR(255) NOT NULL,
    period VARCHAR(10) NOT NULL,
    period_start DATE NOT NULL,
    accumulated_cost DOUBLE PRECISION NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(scope, scope_id, period, period_start)
);

CREATE INDEX IF NOT EXISTS idx_budget_spend_scope ON budget_spend(scope, scope_id, period, period_start);
```

`scope` values: `"global"`, `"tenant"`, `"tenant_model"`.
`scope_id` values: `"global"`, `"premium"`, `"premium:gpt-4o"`.

### Budget Middleware

New middleware `internal/middleware/budget.go` inserted after auth (needs tenant context) and before the handler:

```go
func BudgetCheck(budgetManager *budget.Manager) func(http.Handler) http.Handler
```

1. Extract tenant ID from context
2. Call `budgetManager.Check(tenantID, model)` — returns `(allowed bool, warnings []string, err error)`
3. If not allowed: return 429 with budget details
4. If warnings: add `X-AegisFlow-Budget-Warning` header
5. If allowed: proceed

After request completes, handler calls `budgetManager.RecordSpend(tenantID, model, cost)` to update accumulators.

### Forecast

Every hour, for each budget scope:

```
days_elapsed = now - period_start
daily_rate = accumulated_cost / days_elapsed
projected_total = daily_rate * days_in_period
days_until_exhausted = (budget_limit - accumulated_cost) / daily_rate
```

If `projected_total > budget_limit`, fire an `info` alert: "Tenant premium projected to exceed monthly budget ($500) by $47 at current rate."

### Admin API

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/admin/v1/budgets` | GET | All budget statuses with current spend, percentage, forecast |
| `/admin/v1/budgets/{scope}/{id}` | GET | Specific budget detail with daily breakdown |

#### GET /admin/v1/budgets Response

```json
{
  "global": {
    "monthly_limit": 5000,
    "current_spend": 1250.50,
    "percentage": 25.01,
    "forecast_total": 4800.00,
    "days_remaining": 4,
    "status": "normal"
  },
  "tenants": {
    "premium": {
      "monthly_limit": 500,
      "current_spend": 420.00,
      "percentage": 84.0,
      "forecast_total": 580.00,
      "days_remaining": 2,
      "status": "alert",
      "models": {
        "gpt-4o": {
          "monthly_limit": 200,
          "current_spend": 180.00,
          "percentage": 90.0,
          "forecast_total": 240.00,
          "status": "warn"
        }
      }
    }
  }
}
```

### Dashboard — Budgets Page

Nav item: "Budgets" under Monitoring section.

- Global budget: large progress bar with spend vs limit, forecast line
- Per-tenant cards: spend bar, forecast, model breakdown
- Color coding: green (<80%), yellow (80-90%), orange (90-99%), red (100%)
- Forecast indicator: dashed line showing projected end-of-period spend

### Testing

- Unit tests for budget evaluation: normal, alert threshold, warn threshold, blocked
- Unit tests for forecast: project ahead with known daily rate, edge case (first day of period)
- Unit tests for budget middleware: allowed, warned, blocked
- Unit tests for spend accumulation: record multiple costs, verify totals
- Integration test: configure budget, send requests until budget exceeded, verify 429 response
- Integration test: verify alert fires at alert_at threshold, header added at warn_at

---

## 3D: Multi-Region Routing

### Goal

Allow operators to define provider groups by geographic region with ordered failover between regions.

### Architecture

Extends existing `RouteConfig` with a `regions` field. Each region is an ordered group of providers with its own strategy. The router tries regions in order. Backward compatible — routes without `regions` work exactly as before.

### Components

| Component | Path | Purpose |
|-----------|------|---------|
| Config types | `internal/config/config.go` | `RegionConfig` struct, `Regions` field on `RouteConfig` |
| Router | `internal/router/router.go` | Region-aware routing with per-region strategy and cross-region fallback |
| Dashboard | `internal/admin/dashboard.html` | Region info on Providers and Live Feed pages |
| Request metadata | `internal/admin/requestlog.go` | `Region` field on `RequestEntry` |

### Config

```yaml
providers:
  - name: "openai-us"
    type: "openai"
    enabled: true
    base_url: "https://api.openai.com/v1"
    api_key_env: "OPENAI_US_KEY"
    models: ["gpt-4o"]
    region: "us"

  - name: "azure-eu"
    type: "azure_openai"
    enabled: true
    base_url: "https://eu-gateway.openai.azure.com"
    api_key_env: "AZURE_EU_KEY"
    models: ["gpt-4o"]
    region: "eu"

routes:
  - match:
      model: "gpt-4o"
    regions:
      - name: "us"
        providers: ["openai-us", "azure-us"]
        strategy: "round-robin"
      - name: "eu"
        providers: ["azure-eu"]
        strategy: "priority"
```

#### RegionConfig

```go
type RegionConfig struct {
    Name      string   `yaml:"name"`
    Providers []string `yaml:"providers"`
    Strategy  string   `yaml:"strategy"`
}
```

#### ProviderConfig Addition

```go
type ProviderConfig struct {
    // ... existing fields ...
    Region string `yaml:"region"`
}
```

#### RouteConfig Addition

```go
type RouteConfig struct {
    Match     RouteMatch     `yaml:"match"`
    Providers []string       `yaml:"providers"`
    Strategy  string         `yaml:"strategy"`
    Canary    *CanaryConfig  `yaml:"canary,omitempty"`
    Regions   []RegionConfig `yaml:"regions,omitempty"`
}
```

### Router Logic

Current flow:
```
match route → iterate providers with strategy → fallback on circuit break
```

New flow:
```
match route → if regions defined:
    for each region in order:
        iterate region's providers with region's strategy
        if any provider succeeds → return response, tag request with region
        if all providers circuit-broken → continue to next region
    all regions exhausted → return 502
else:
    existing behavior (flat providers list)
```

### Backward Compatibility

- Routes with `providers` + `strategy` (no `regions`): work exactly as before
- Routes with `regions` defined: `regions` takes precedence, `providers` and `strategy` at route level are ignored
- If both `regions` and `providers` specified: log warning at startup, use `regions`

### Request Metadata

Add `Region` field to `RequestEntry` in `requestlog.go`:

```go
type RequestEntry struct {
    // ... existing fields ...
    Region    string `json:"region,omitempty"`
    Provider  string `json:"provider,omitempty"`
}
```

### Dashboard Changes

- **Providers page**: show region tag next to each provider
- **Live Feed page**: show region column in request table
- **Dashboard overview**: provider status grouped by region

### Testing

- Unit tests for region-aware routing: first region succeeds, first region fails falls to second, all regions fail returns 502
- Unit tests for backward compatibility: route without regions uses existing logic
- Unit tests for config validation: warn on both regions + providers specified
- Integration test: configure 2 regions, circuit-break all providers in region 1, verify failover to region 2

---

## 3E: Kubernetes Operator with CRDs

### Goal

Allow operators to manage AegisFlow configuration as Kubernetes-native resources with live status reporting via `kubectl`.

### Architecture

A separate binary `cmd/aegisflow-operator/` using `controller-runtime`. Defines CRDs for AegisFlow resources. Operator watches CRDs, generates `aegisflow.yaml` into a ConfigMap, and writes status back to CRD objects by reading the gateway's admin API.

### Components

| Component | Path | Purpose |
|-----------|------|---------|
| `main.go` | `cmd/aegisflow-operator/` | Operator entrypoint, manager setup |
| `reconciler.go` | `internal/operator/` | Main reconciliation: CRDs → ConfigMap |
| `status.go` | `internal/operator/` | Status reporter: reads gateway admin API, writes CRD status |
| CRD types | `api/v1alpha1/` | Go type definitions for all CRDs |
| `zz_generated.deepcopy.go` | `api/v1alpha1/` | Generated deep copy methods |
| CRD manifests | `deployments/crds/` | YAML CRD definitions |
| RBAC | `deployments/operator/` | ServiceAccount, ClusterRole, ClusterRoleBinding |
| Helm chart update | `deployments/helm/aegisflow/` | Optional operator deployment + CRD templates |

### CRDs

#### AegisFlowGateway

```yaml
apiVersion: aegisflow.io/v1alpha1
kind: AegisFlowGateway
metadata:
  name: aegisflow
  namespace: default
spec:
  server:
    port: 8080
    adminPort: 8081
  logging:
    level: info
    format: json
  telemetry:
    enabled: true
    exporter: otlp
  cache:
    enabled: true
    backend: memory
    ttl: 5m
    maxSize: 1000
  database:
    enabled: true
    connStringSecret:
      name: aegisflow-db
      key: connection-string
  admin:
    tokenSecret:
      name: aegisflow-admin
      key: token
status:
  ready: true
  totalRequests: 145892
  activeTenants: 3
  lastSyncedAt: "2026-03-27T10:00:00Z"
```

#### AegisFlowProvider

```yaml
apiVersion: aegisflow.io/v1alpha1
kind: AegisFlowProvider
metadata:
  name: openai-us
spec:
  type: openai
  baseURL: "https://api.openai.com/v1"
  apiKeySecret:
    name: openai-credentials
    key: api-key
  models:
    - gpt-4o
    - gpt-4o-mini
  timeout: 60s
  maxRetries: 2
  region: us
status:
  healthy: true
  averageLatencyMs: 450
  errorRate: 0.5
  lastCheckedAt: "2026-03-27T10:00:00Z"
```

#### AegisFlowRoute

```yaml
apiVersion: aegisflow.io/v1alpha1
kind: AegisFlowRoute
metadata:
  name: gpt4o-route
spec:
  match:
    model: "gpt-4o"
  regions:
    - name: us
      providers: ["openai-us", "azure-us"]
      strategy: round-robin
    - name: eu
      providers: ["azure-eu"]
      strategy: priority
  canary:
    targetProvider: azure-us
    stages: [5, 25, 50, 100]
    observationWindow: 5m
    errorThreshold: 5.0
    latencyP95Threshold: 3000
status:
  activeCanary: true
  canaryPercentage: 25
  canaryHealth: healthy
```

#### AegisFlowTenant

```yaml
apiVersion: aegisflow.io/v1alpha1
kind: AegisFlowTenant
metadata:
  name: premium
spec:
  displayName: "Premium Tenant"
  apiKeySecrets:
    - name: premium-keys
      key: key-1
    - name: premium-keys
      key: key-2
  rateLimit:
    requestsPerMinute: 300
    tokensPerMinute: 500000
  allowedModels: ["*"]
  budget:
    monthly: 500
    alertAt: 80
    warnAt: 90
    models:
      gpt-4o:
        monthly: 200
status:
  currentUsage:
    totalRequests: 45000
    totalTokens: 12000000
    estimatedCost: 380.50
  budgetPercentage: 76.1
  rateLimitUtilization: 42
```

#### AegisFlowPolicy

```yaml
apiVersion: aegisflow.io/v1alpha1
kind: AegisFlowPolicy
metadata:
  name: block-jailbreak
spec:
  phase: input
  type: keyword
  action: block
  keywords:
    - "ignore previous instructions"
    - "DAN mode"
    - "jailbreak"
---
apiVersion: aegisflow.io/v1alpha1
kind: AegisFlowPolicy
metadata:
  name: custom-toxicity
spec:
  phase: input
  type: wasm
  action: block
  wasmPath: /plugins/toxicity.wasm
  timeout: 100ms
  onError: block
```

### Reconciliation Flow

1. Operator watches all 5 CRD types for changes
2. On any change, reconciler runs:
   a. List all `AegisFlowProvider` resources → build providers config
   b. List all `AegisFlowRoute` resources → build routes config
   c. List all `AegisFlowTenant` resources → build tenants config, resolve Secret references to env var names
   d. List all `AegisFlowPolicy` resources → build policies config
   e. Read `AegisFlowGateway` resource → build server/logging/telemetry/cache/database config, resolve Secret references
   f. Marshal complete config to YAML
   g. Create/update ConfigMap `aegisflow-config` with the generated YAML
3. Gateway pod mounts ConfigMap as volume → file change triggers hot-reload
4. Reconciler sets `.status.lastSyncedAt` on `AegisFlowGateway`

### Status Reporting Flow

Runs as a separate goroutine, every 30 seconds:

1. Call gateway health endpoint: `GET http://aegisflow-service:8081/health`
2. Call usage endpoint: `GET http://aegisflow-service:8081/admin/v1/usage`
3. Call providers endpoint: `GET http://aegisflow-service:8081/admin/v1/providers`
4. Call rollouts endpoint: `GET http://aegisflow-service:8081/admin/v1/rollouts`
5. Call budgets endpoint: `GET http://aegisflow-service:8081/admin/v1/budgets`
6. Update status on each CRD object with relevant data

### Secret Handling

CRDs reference Kubernetes Secrets for sensitive values (API keys, database passwords, admin tokens). The operator:

1. Reads the Secret value
2. Sets it as an environment variable in the gateway Deployment's pod spec (via env from Secret ref)
3. Writes the env var name into the generated config

This way, secrets never appear in the ConfigMap. The gateway reads them from environment variables as it does today.

### kubectl Output

```
$ kubectl get aegisflowgateways
NAME        READY   REQUESTS   TENANTS   SYNCED
aegisflow   true    145892     3         2m ago

$ kubectl get aegisflowproviders
NAME          TYPE          REGION   HEALTHY   LATENCY   ERROR-RATE
openai-us     openai        us       true      450ms     0.5%
azure-eu      azure_openai  eu       true      380ms     0.2%
ollama-local  ollama        local    true      120ms     0.0%

$ kubectl get aegisflowroutes
NAME          MODEL      STRATEGY      CANARY   HEALTH
gpt4o-route   gpt-4o     round-robin   25%      healthy
claude-route  claude-*   priority      -        healthy

$ kubectl get aegisflowtenants
NAME      REQUESTS   BUDGET    RATE-LIMIT
premium   45000      76.1%     42%
default   98000      25.0%     15%

$ kubectl get aegisflowpolicies
NAME              PHASE   TYPE      ACTION
block-jailbreak   input   keyword   block
pii-detection     input   pii       warn
custom-toxicity   input   wasm      block
```

### Dependencies

| Package | Purpose |
|---------|---------|
| `sigs.k8s.io/controller-runtime` | Kubernetes operator framework |
| `k8s.io/apimachinery` | K8s API types and machinery |
| `k8s.io/client-go` | K8s client for Secret reads |

### Helm Chart Updates

Add to `values.yaml`:

```yaml
operator:
  enabled: false
  image:
    repository: aegisflow-operator
    tag: latest
  resources:
    limits:
      cpu: 200m
      memory: 128Mi

crds:
  install: true
```

When `operator.enabled: true`, Helm installs:
- CRD definitions
- Operator Deployment
- ServiceAccount + ClusterRole + ClusterRoleBinding
- The gateway Deployment switches from ConfigMap volume mount (from file) to ConfigMap volume mount (from operator-managed ConfigMap)

### Testing

- Unit tests for reconciler: CRDs → generated YAML matches expected config
- Unit tests for Secret resolution: correct env var names generated
- Unit tests for status reporter: mock admin API responses → correct CRD status updates
- Integration test (envtest): create CRDs, verify ConfigMap generated, update CRD, verify ConfigMap updated
- Integration test (envtest): delete a provider CRD, verify it's removed from generated config

---

## Dashboard Updates Summary

Phase 3 adds 4 new dashboard pages:

| Page | Nav Section | Sub-project |
|------|-------------|-------------|
| Rollouts | Monitoring | 3A |
| Analytics | Monitoring | 3B |
| Alerts | Monitoring | 3B |
| Budgets | Monitoring | 3C |

Updated existing pages:
- **Providers**: show region tags (3D)
- **Live Feed**: show region and provider columns (3D)

Final sidebar nav structure:

```
OVERVIEW
  Dashboard

MONITORING
  Policies
  Live Feed
  Violations
  Cache
  Rollouts        [NEW]
  Analytics       [NEW]
  Alerts          [NEW]
  Budgets         [NEW]

CONFIGURATION
  Providers
  Tenants
```

---

## Implementation Order

Each sub-project is a standalone spec → plan → implementation cycle:

1. **3A: Canary Rollouts** — ~15 files, extends router + new rollout package + API + dashboard
2. **3B: Analytics & Anomaly Detection** — ~10 files, new analytics package + API + dashboard
3. **3C: Cost Forecasting & Budgets** — ~8 files, new budget package + middleware + API + dashboard
4. **3D: Multi-Region Routing** — ~6 files, extends config + router + dashboard
5. **3E: Kubernetes Operator** — ~12 files, new operator binary + CRD types + manifests + Helm updates
