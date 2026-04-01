package analytics

import (
	"testing"
	"time"
)

func TestAdminAdapterRealtimeSummary(t *testing.T) {
	c := NewCollector(1)
	am := NewAlertManager(nil)
	adapter := NewAdminAdapter(c, am)

	c.Record(DataPoint{TenantID: "t1", Model: "m1", Provider: "p1", StatusCode: 200, LatencyMs: 100, Timestamp: time.Now()})

	summary := adapter.RealtimeSummary()
	if len(summary) == 0 {
		t.Error("expected non-empty summary")
	}
	if _, ok := summary["global"]; !ok {
		t.Error("expected global dimension")
	}
}

func TestAdminAdapterRecentAlerts(t *testing.T) {
	c := NewCollector(1)
	am := NewAlertManager(nil)
	adapter := NewAdminAdapter(c, am)

	// Process an alert
	am.ProcessAlerts(DetectionResult{
		Alerts: []Alert{{
			ID: "a1", Severity: SeverityCritical, Type: "static_threshold",
			Dimension: "global", Metric: "error_rate", Value: 50, Threshold: 20,
			Message: "test", State: "active", CreatedAt: time.Now(),
		}},
	})

	result := adapter.RecentAlerts(10)
	alerts, ok := result.([]*Alert)
	if !ok {
		t.Fatal("expected []*Alert")
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
}

func TestAdminAdapterAcknowledgeAlert(t *testing.T) {
	c := NewCollector(1)
	am := NewAlertManager(nil)
	adapter := NewAdminAdapter(c, am)

	am.ProcessAlerts(DetectionResult{
		Alerts: []Alert{{
			ID: "a1", Severity: SeverityCritical, Type: "static_threshold",
			Dimension: "global", Metric: "error_rate", Value: 50, Threshold: 20,
			Message: "test", State: "active", CreatedAt: time.Now(),
		}},
	})

	if !adapter.AcknowledgeAlert("a1") {
		t.Error("expected successful acknowledge")
	}
	if adapter.AcknowledgeAlert("nonexistent") {
		t.Error("expected false for nonexistent alert")
	}
}

func TestAdminAdapterDimensions(t *testing.T) {
	c := NewCollector(1)
	am := NewAlertManager(nil)
	adapter := NewAdminAdapter(c, am)

	c.Record(DataPoint{TenantID: "t1", Model: "m1", Provider: "p1", StatusCode: 200, LatencyMs: 100, Timestamp: time.Now()})

	dims := adapter.Dimensions()
	if len(dims) != 4 {
		t.Errorf("expected 4 dimensions, got %d", len(dims))
	}
}
