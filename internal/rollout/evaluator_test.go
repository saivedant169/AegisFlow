package rollout

import (
	"testing"
	"time"

	"github.com/saivedant169/AegisFlow/internal/admin"
)

func newTestRollout() *Rollout {
	return &Rollout{
		RouteModel:          "gpt-4o",
		CanaryProvider:      "azure",
		State:               StateRunning,
		ErrorThreshold:      5.0,
		LatencyP95Threshold: 3000,
		ObservationWindow:   1 * time.Second,
		StageStartedAt:      time.Now().Add(-2 * time.Second),
	}
}

func addEntries(rl *admin.RequestLog, n int, provider string, status int, latencyMs int64) {
	for i := 0; i < n; i++ {
		rl.Add(admin.RequestEntry{
			Timestamp: time.Now().Add(-time.Duration(i) * time.Millisecond),
			Model:     "gpt-4o",
			Provider:  provider,
			Status:    status,
			LatencyMs: latencyMs,
		})
	}
}

func TestEvaluatePromotesHealthyCanary(t *testing.T) {
	rl := admin.NewRequestLog(200)
	addEntries(rl, 80, "openai", 200, 500)
	addEntries(rl, 20, "azure", 200, 500)

	ev := NewEvaluator(rl)
	decision, reason, metrics := ev.Evaluate(newTestRollout())

	if decision != "promote" {
		t.Fatalf("expected promote, got %s: %s", decision, reason)
	}
	if metrics.Canary.Requests != 20 {
		t.Fatalf("expected 20 canary requests, got %d", metrics.Canary.Requests)
	}
	if metrics.Baseline.Requests != 80 {
		t.Fatalf("expected 80 baseline requests, got %d", metrics.Baseline.Requests)
	}
	if reason != "canary healthy" {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestEvaluateRollsBackHighErrorRate(t *testing.T) {
	rl := admin.NewRequestLog(200)
	addEntries(rl, 9, "azure", 200, 500)
	addEntries(rl, 3, "azure", 500, 500)

	ev := NewEvaluator(rl)
	decision, _, metrics := ev.Evaluate(newTestRollout())

	if decision != "rollback" {
		t.Fatalf("expected rollback, got %s", decision)
	}
	if metrics.Canary.ErrorRate != 25.0 {
		t.Fatalf("expected 25%% error rate, got %.1f%%", metrics.Canary.ErrorRate)
	}
}

func TestEvaluateRollsBackHighLatency(t *testing.T) {
	rl := admin.NewRequestLog(200)
	addEntries(rl, 20, "azure", 200, 4000)

	ev := NewEvaluator(rl)
	decision, _, metrics := ev.Evaluate(newTestRollout())

	if decision != "rollback" {
		t.Fatalf("expected rollback, got %s", decision)
	}
	if metrics.Canary.P95LatencyMs != 4000 {
		t.Fatalf("expected p95 4000ms, got %dms", metrics.Canary.P95LatencyMs)
	}
}

func TestEvaluateWaitsForMinRequests(t *testing.T) {
	rl := admin.NewRequestLog(200)
	addEntries(rl, 5, "azure", 200, 500)

	ev := NewEvaluator(rl)
	decision, reason, _ := ev.Evaluate(newTestRollout())

	if decision != "wait" {
		t.Fatalf("expected wait, got %s", decision)
	}
	if reason != "insufficient canary requests" {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestEvaluateWaitsForObservationWindow(t *testing.T) {
	rl := admin.NewRequestLog(200)
	addEntries(rl, 20, "azure", 200, 500)

	r := newTestRollout()
	r.StageStartedAt = time.Now() // just started, window not elapsed

	ev := NewEvaluator(rl)
	decision, reason, _ := ev.Evaluate(r)

	if decision != "wait" {
		t.Fatalf("expected wait, got %s", decision)
	}
	if reason != "observation window not elapsed" {
		t.Fatalf("unexpected reason: %s", reason)
	}
}

func TestCalculateHealthEmpty(t *testing.T) {
	h := calculateHealth(nil)
	if h.ErrorRate != 0 || h.P95LatencyMs != 0 || h.Requests != 0 {
		t.Fatalf("expected zero metrics, got %+v", h)
	}
}
