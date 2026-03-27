# Phase 3B: Advanced Analytics & Anomaly Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide real-time and historical analytics with automatic anomaly detection for traffic spikes, performance degradation, and cost anomalies.

**Architecture:** A new `internal/analytics/` package with an in-memory time-series collector (1-min buckets, 48h rolling window), a statistical anomaly detector (static thresholds + moving average baseline), and an alert system that fires webhooks and feeds the dashboard. Hourly aggregates flush to PostgreSQL for historical queries.

**Tech Stack:** Go, PostgreSQL, Chart.js (dashboard)

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/analytics/collector.go` | Create | In-memory time-series ring buffer, per-dimension metrics |
| `internal/analytics/detector.go` | Create | Static threshold + statistical baseline anomaly detection |
| `internal/analytics/alerts.go` | Create | Alert lifecycle: create, resolve. Webhook integration |
| `internal/analytics/store.go` | Create | PostgreSQL tables for aggregates + alert history |
| `internal/analytics/collector_test.go` | Create | Tests for collector bucketing and ring buffer |
| `internal/analytics/detector_test.go` | Create | Tests for threshold and baseline detection |
| `internal/admin/admin.go` | Modify | Add analytics/alerts API endpoints |
| `internal/admin/dashboard.html` | Modify | Analytics + Alerts dashboard pages |
| `internal/gateway/handler.go` | Modify | Feed data points to analytics collector |
| `internal/config/config.go` | Modify | Add AnalyticsConfig |
| `cmd/aegisflow/main.go` | Modify | Initialize analytics system |

---

### Task 1: Add analytics config and collector types

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/analytics/collector.go`

Add `AnalyticsConfig` to config and create the in-memory time-series collector with 1-minute buckets, 48h retention, per-dimension (tenant/model/provider) tracking.

---

### Task 2: Implement anomaly detector

**Files:**
- Create: `internal/analytics/detector.go`
- Create: `internal/analytics/detector_test.go`

Static thresholds (error_rate_max, p95_latency_max, requests_per_minute_max, cost_per_minute_max) + statistical baseline (24h moving avg + stddev, alert when >N stddev deviation).

---

### Task 3: Implement alert system

**Files:**
- Create: `internal/analytics/alerts.go`
- Create: `internal/analytics/store.go`

Alert lifecycle (active/resolved), severity levels (critical/warning/info), auto-resolve after 5 consecutive normal minutes. PostgreSQL persistence for alert history. Webhook firing on new alerts.

---

### Task 4: Wire analytics into gateway handler

**Files:**
- Modify: `internal/gateway/handler.go`
- Modify: `cmd/aegisflow/main.go`

After each request, call `collector.Record(DataPoint{...})`. Initialize collector and detector in main.go. Start background detection goroutine.

---

### Task 5: Add analytics and alerts admin API

**Files:**
- Modify: `internal/admin/admin.go`

Endpoints: GET /admin/v1/analytics (aggregated), GET /admin/v1/analytics/realtime (current 5-min), GET /admin/v1/alerts (list), POST /admin/v1/alerts/{id}/acknowledge.

---

### Task 6: Add Analytics + Alerts dashboard pages

**Files:**
- Modify: `internal/admin/dashboard.html`

Analytics page: time-series charts for request rate, error rate, latency. Alerts page: active/resolved alerts with severity badges.

---

### Task 7: End-to-end verification

Full test suite + smoke test.
