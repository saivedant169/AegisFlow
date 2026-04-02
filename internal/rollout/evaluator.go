package rollout

import (
	"fmt"
	"sort"
	"time"

	"github.com/saivedant169/AegisFlow/internal/admin"
)

const minRequestsForDecision = 10

type Evaluator struct {
	requestLog *admin.RequestLog
}

func NewEvaluator(reqLog *admin.RequestLog) *Evaluator {
	return &Evaluator{requestLog: reqLog}
}

func (e *Evaluator) Evaluate(r *Rollout) (decision string, reason string, metrics RolloutMetrics) {
	if r.State != StateRunning {
		return "wait", "rollout not running", metrics
	}
	if time.Since(r.StageStartedAt) < r.ObservationWindow {
		return "wait", "observation window not elapsed", metrics
	}

	entries := e.requestLog.Recent(e.requestLog.Count())
	cutoff := time.Now().Add(-r.ObservationWindow)

	var baselineEntries, canaryEntries []admin.RequestEntry
	for _, entry := range entries {
		if entry.Timestamp.Before(cutoff) || entry.Model != r.RouteModel {
			continue
		}
		if entry.Provider == r.CanaryProvider {
			canaryEntries = append(canaryEntries, entry)
		} else {
			baselineEntries = append(baselineEntries, entry)
		}
	}

	metrics.Baseline = calculateHealth(baselineEntries)
	metrics.Canary = calculateHealth(canaryEntries)

	if metrics.Canary.Requests < minRequestsForDecision {
		return "wait", "insufficient canary requests", metrics
	}
	if metrics.Canary.ErrorRate > r.ErrorThreshold {
		return "rollback", fmt.Sprintf("canary error rate %.1f%% exceeds threshold %.1f%%", metrics.Canary.ErrorRate, r.ErrorThreshold), metrics
	}
	if metrics.Canary.P95LatencyMs > r.LatencyP95Threshold {
		return "rollback", fmt.Sprintf("canary p95 latency %dms exceeds threshold %dms", metrics.Canary.P95LatencyMs, r.LatencyP95Threshold), metrics
	}
	return "promote", "canary healthy", metrics
}

func calculateHealth(entries []admin.RequestEntry) HealthMetrics {
	if len(entries) == 0 {
		return HealthMetrics{}
	}
	var errors int64
	latencies := make([]int64, 0, len(entries))
	for _, e := range entries {
		if e.Status >= 500 {
			errors++
		}
		latencies = append(latencies, e.LatencyMs)
	}
	total := int64(len(entries))
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95Idx := int(float64(len(latencies)) * 0.95)
	if p95Idx >= len(latencies) {
		p95Idx = len(latencies) - 1
	}
	return HealthMetrics{
		ErrorRate:    float64(errors) / float64(total) * 100,
		P95LatencyMs: latencies[p95Idx],
		Requests:     total,
	}
}
