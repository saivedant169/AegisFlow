package analytics

import (
	"testing"
	"time"
)

func TestAlertManagerProcessNewAlert(t *testing.T) {
	am := NewAlertManager(nil) // no webhook

	result := DetectionResult{
		Alerts: []Alert{
			{
				ID:        "alert-1",
				Severity:  SeverityCritical,
				Type:      "static_threshold",
				Dimension: "global",
				Metric:    "error_rate",
				Value:     50,
				Threshold: 20,
				Message:   "error rate too high",
				State:     "active",
				CreatedAt: time.Now(),
			},
		},
	}

	am.ProcessAlerts(result)

	active := am.ActiveAlerts()
	if len(active) != 1 {
		t.Fatalf("expected 1 active alert, got %d", len(active))
	}
	if active[0].Metric != "error_rate" {
		t.Errorf("expected error_rate metric, got %s", active[0].Metric)
	}
}

func TestAlertManagerDeduplicates(t *testing.T) {
	am := NewAlertManager(nil)

	alert := Alert{
		ID: "alert-1", Severity: SeverityCritical, Type: "static_threshold",
		Dimension: "global", Metric: "error_rate", Value: 50, Threshold: 20,
		Message: "error rate too high", State: "active", CreatedAt: time.Now(),
	}

	// Process the same alert twice.
	am.ProcessAlerts(DetectionResult{Alerts: []Alert{alert}})
	am.ProcessAlerts(DetectionResult{Alerts: []Alert{alert}})

	active := am.ActiveAlerts()
	if len(active) != 1 {
		t.Fatalf("expected 1 active alert (deduplication), got %d", len(active))
	}
}

func TestAlertManagerAutoResolve(t *testing.T) {
	am := NewAlertManager(nil)

	alert := Alert{
		ID: "alert-1", Severity: SeverityCritical, Type: "static_threshold",
		Dimension: "global", Metric: "error_rate", Value: 50, Threshold: 20,
		Message: "error rate too high", State: "active", CreatedAt: time.Now(),
	}

	// Fire the alert once.
	am.ProcessAlerts(DetectionResult{Alerts: []Alert{alert}})

	// Send 5 empty evaluations to trigger auto-resolve (resolveAfter=5).
	for i := 0; i < 5; i++ {
		am.ProcessAlerts(DetectionResult{Alerts: []Alert{}})
	}

	active := am.ActiveAlerts()
	if len(active) != 0 {
		t.Fatalf("expected 0 active alerts after auto-resolve, got %d", len(active))
	}

	recent := am.RecentAlerts(10)
	if len(recent) != 1 {
		t.Fatalf("expected 1 resolved alert in history, got %d", len(recent))
	}
	if recent[0].State != "resolved" {
		t.Errorf("expected resolved state, got %s", recent[0].State)
	}
}

func TestAlertManagerAcknowledge(t *testing.T) {
	am := NewAlertManager(nil)

	alert := Alert{
		ID: "alert-1", Severity: SeverityCritical, Type: "static_threshold",
		Dimension: "global", Metric: "error_rate", Value: 50, Threshold: 20,
		Message: "error rate too high", State: "active", CreatedAt: time.Now(),
	}

	am.ProcessAlerts(DetectionResult{Alerts: []Alert{alert}})

	if !am.Acknowledge("alert-1") {
		t.Error("expected Acknowledge to return true for existing alert")
	}

	active := am.ActiveAlerts()
	if len(active) != 1 {
		t.Fatal("acknowledged alert should still be active")
	}
	if active[0].State != "acknowledged" {
		t.Errorf("expected acknowledged state, got %s", active[0].State)
	}

	// Acknowledge nonexistent alert.
	if am.Acknowledge("nonexistent") {
		t.Error("expected Acknowledge to return false for nonexistent alert")
	}
}

func TestAlertManagerRecentAlertsLimit(t *testing.T) {
	am := NewAlertManager(nil)

	// Create multiple distinct alerts.
	for i := 0; i < 5; i++ {
		am.ProcessAlerts(DetectionResult{
			Alerts: []Alert{{
				ID: "alert", Severity: SeverityCritical, Type: "static_threshold",
				Dimension: "global", Metric: "metric_" + string(rune('a'+i)),
				Value: 50, Threshold: 20, Message: "alert", State: "active",
				CreatedAt: time.Now(),
			}},
		})
	}

	recent := am.RecentAlerts(2)
	if len(recent) != 2 {
		t.Fatalf("expected 2 recent alerts with limit, got %d", len(recent))
	}
}
