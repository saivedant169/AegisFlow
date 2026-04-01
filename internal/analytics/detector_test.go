package analytics

import (
	"testing"
	"time"
)

func TestStaticThresholdErrorRate(t *testing.T) {
	c := NewCollector(24)
	now := time.Now().Truncate(time.Minute)

	// Record 10 requests, 5 with 500 status -> 50% error rate
	for i := 0; i < 10; i++ {
		status := 200
		if i < 5 {
			status = 500
		}
		c.Record(DataPoint{
			TenantID:   "t1",
			Model:      "gpt-4",
			Provider:   "openai",
			StatusCode: status,
			LatencyMs:  100,
			Tokens:     10,
			Timestamp:  now,
		})
	}

	d := NewDetector(c, StaticThresholds{
		ErrorRateMax: 20, // 20%
	}, BaselineConfig{})

	result := d.Evaluate()

	// Should fire error_rate alerts for multiple dimensions (tenant, model, provider, global)
	found := false
	for _, a := range result.Alerts {
		if a.Metric == "error_rate" && a.Type == "static_threshold" {
			found = true
			if a.Severity != SeverityCritical {
				t.Errorf("expected critical severity, got %s", a.Severity)
			}
			if a.Value != 50 {
				t.Errorf("expected error rate 50%%, got %.1f%%", a.Value)
			}
		}
	}
	if !found {
		t.Fatal("expected at least one error_rate static threshold alert")
	}
}

func TestStaticThresholdNoAlert(t *testing.T) {
	c := NewCollector(24)
	now := time.Now().Truncate(time.Minute)

	// Record 10 healthy requests
	for i := 0; i < 10; i++ {
		c.Record(DataPoint{
			TenantID:      "t1",
			Model:         "gpt-4",
			Provider:      "openai",
			StatusCode:    200,
			LatencyMs:     50,
			Tokens:        10,
			EstimatedCost: 0.01,
			Timestamp:     now,
		})
	}

	d := NewDetector(c, StaticThresholds{
		ErrorRateMax:         20,
		P95LatencyMax:        5000,
		RequestsPerMinuteMax: 100,
		CostPerMinuteMax:     50,
	}, BaselineConfig{})

	result := d.Evaluate()
	if len(result.Alerts) != 0 {
		t.Fatalf("expected no alerts, got %d: %+v", len(result.Alerts), result.Alerts)
	}
}

func TestBaselineAnomalyDetected(t *testing.T) {
	c := NewCollector(24)
	baseTime := time.Now().Add(-90 * time.Minute).Truncate(time.Minute)

	// Record 80 minutes of steady traffic: ~10 requests per minute with slight variance
	for m := 0; m < 80; m++ {
		ts := baseTime.Add(time.Duration(m) * time.Minute)
		// Alternate between 9 and 11 requests to produce non-zero stddev
		count := 10
		if m%2 == 0 {
			count = 9
		} else {
			count = 11
		}
		for r := 0; r < count; r++ {
			c.Record(DataPoint{
				TenantID:   "t1",
				Model:      "gpt-4",
				Provider:   "openai",
				StatusCode: 200,
				LatencyMs:  100,
				Tokens:     10,
				Timestamp:  ts,
			})
		}
	}

	// Spike: last 5 minutes at 100 requests per minute
	for m := 80; m < 85; m++ {
		ts := baseTime.Add(time.Duration(m) * time.Minute)
		for r := 0; r < 100; r++ {
			c.Record(DataPoint{
				TenantID:   "t1",
				Model:      "gpt-4",
				Provider:   "openai",
				StatusCode: 200,
				LatencyMs:  100,
				Tokens:     10,
				Timestamp:  ts,
			})
		}
	}

	d := NewDetector(c, StaticThresholds{}, BaselineConfig{
		Window:          2 * time.Hour,
		StddevThreshold: 3,
	})

	result := d.Evaluate()

	found := false
	for _, a := range result.Alerts {
		if a.Type == "statistical_baseline" && a.Metric == "request_rate" {
			found = true
			if a.Severity != SeverityWarning {
				t.Errorf("expected warning severity, got %s", a.Severity)
			}
		}
	}
	if !found {
		t.Fatal("expected at least one baseline request_rate anomaly alert")
	}
}

func TestBaselineInsufficientData(t *testing.T) {
	c := NewCollector(24)
	baseTime := time.Now().Truncate(time.Minute)

	// Only 30 minutes of data -- below the 60-bucket minimum
	for m := 0; m < 30; m++ {
		ts := baseTime.Add(time.Duration(m) * time.Minute)
		c.Record(DataPoint{
			TenantID:   "t1",
			Model:      "gpt-4",
			Provider:   "openai",
			StatusCode: 200,
			LatencyMs:  100,
			Tokens:     10,
			Timestamp:  ts,
		})
	}

	d := NewDetector(c, StaticThresholds{}, BaselineConfig{
		Window:          2 * time.Hour,
		StddevThreshold: 3,
	})

	result := d.Evaluate()

	for _, a := range result.Alerts {
		if a.Type == "statistical_baseline" {
			t.Fatalf("expected no baseline alerts with insufficient data, got: %+v", a)
		}
	}
}

func TestStaticThresholdAllFourMetrics(t *testing.T) {
	c := NewCollector(24)
	now := time.Now().Truncate(time.Minute)

	// Record data that exceeds all 4 thresholds:
	// - high error rate (all 500s)
	// - high p95 latency (10000ms)
	// - high request count (20 requests in 1 minute)
	// - high cost ($5 per request)
	for i := 0; i < 20; i++ {
		c.Record(DataPoint{
			TenantID:      "t1",
			Model:         "gpt-4",
			Provider:      "openai",
			StatusCode:    500,
			LatencyMs:     10000,
			Tokens:        100,
			EstimatedCost: 5.0,
			Timestamp:     now,
		})
	}

	d := NewDetector(c, StaticThresholds{
		ErrorRateMax:         10,   // 10%, actual is 100%
		P95LatencyMax:        5000, // 5000ms, actual is 10000ms
		RequestsPerMinuteMax: 10,   // 10/min, actual is 20
		CostPerMinuteMax:     50,   // $50/min, actual is $100
	}, BaselineConfig{})

	result := d.Evaluate()

	// We should find alerts for all 4 metrics across at least one dimension.
	metricsFound := map[string]bool{}
	for _, a := range result.Alerts {
		if a.Type == "static_threshold" {
			metricsFound[a.Metric] = true
		}
	}

	for _, metric := range []string{"error_rate", "p95_latency", "request_rate", "cost_rate"} {
		if !metricsFound[metric] {
			t.Errorf("expected static threshold alert for %s, but none found", metric)
		}
	}
}

func TestBaselineExactly60Buckets(t *testing.T) {
	c := NewCollector(24)
	baseTime := time.Now().Add(-70 * time.Minute).Truncate(time.Minute)

	// Record exactly 60 minutes of steady traffic (boundary case for the >= 60 check).
	for m := 0; m < 55; m++ {
		ts := baseTime.Add(time.Duration(m) * time.Minute)
		count := 10
		if m%2 == 0 {
			count = 8
		} else {
			count = 12
		}
		for r := 0; r < count; r++ {
			c.Record(DataPoint{
				TenantID:   "t1",
				Model:      "gpt-4",
				Provider:   "openai",
				StatusCode: 200,
				LatencyMs:  100,
				Tokens:     10,
				Timestamp:  ts,
			})
		}
	}

	// Spike in last 5 minutes
	for m := 55; m < 60; m++ {
		ts := baseTime.Add(time.Duration(m) * time.Minute)
		for r := 0; r < 200; r++ {
			c.Record(DataPoint{
				TenantID:   "t1",
				Model:      "gpt-4",
				Provider:   "openai",
				StatusCode: 200,
				LatencyMs:  100,
				Tokens:     10,
				Timestamp:  ts,
			})
		}
	}

	d := NewDetector(c, StaticThresholds{}, BaselineConfig{
		Window:          2 * time.Hour,
		StddevThreshold: 3,
	})

	result := d.Evaluate()

	found := false
	for _, a := range result.Alerts {
		if a.Type == "statistical_baseline" && a.Metric == "request_rate" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected baseline anomaly alert with exactly 60 buckets of data")
	}
}

func TestDetectorEvaluateMultipleDimensions(t *testing.T) {
	c := NewCollector(24)
	now := time.Now().Truncate(time.Minute)

	// Tenant A: high error rate
	for i := 0; i < 10; i++ {
		c.Record(DataPoint{
			TenantID:   "tenantA",
			Model:      "gpt-4",
			Provider:   "openai",
			StatusCode: 500,
			LatencyMs:  100,
			Tokens:     10,
			Timestamp:  now,
		})
	}

	// Tenant B: healthy
	for i := 0; i < 10; i++ {
		c.Record(DataPoint{
			TenantID:   "tenantB",
			Model:      "gpt-3.5",
			Provider:   "openai",
			StatusCode: 200,
			LatencyMs:  50,
			Tokens:     5,
			Timestamp:  now,
		})
	}

	d := NewDetector(c, StaticThresholds{
		ErrorRateMax: 20,
	}, BaselineConfig{})

	result := d.Evaluate()

	tenantAAlerts := 0
	tenantBAlerts := 0
	for _, a := range result.Alerts {
		if a.Dimension == "tenant:tenantA" {
			tenantAAlerts++
		}
		if a.Dimension == "tenant:tenantB" {
			tenantBAlerts++
		}
	}

	if tenantAAlerts == 0 {
		t.Error("expected alerts for tenantA (high error rate)")
	}
	if tenantBAlerts != 0 {
		t.Errorf("expected no alerts for tenantB, got %d", tenantBAlerts)
	}
}
