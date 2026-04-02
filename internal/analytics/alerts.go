package analytics

import (
	"log"
	"sync"
	"time"

	"github.com/saivedant169/AegisFlow/internal/webhook"
)

type AlertManager struct {
	mu             sync.RWMutex
	active         map[string]*Alert  // keyed by dimension+metric
	history        []*Alert           // resolved alerts
	maxHistory     int
	webhook        *webhook.Notifier
	normalCounters map[string]int     // consecutive normal evaluations per dimension+metric
	resolveAfter   int                // consecutive normal evals before auto-resolve
}

func NewAlertManager(wh *webhook.Notifier) *AlertManager {
	return &AlertManager{
		active:         make(map[string]*Alert),
		maxHistory:     500,
		webhook:        wh,
		normalCounters: make(map[string]int),
		resolveAfter:   5,
	}
}

func alertKey(dimension, metric string) string {
	return dimension + ":" + metric
}

// ProcessAlerts takes detection results and manages alert lifecycle.
func (am *AlertManager) ProcessAlerts(result DetectionResult) {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Track which keys had alerts this cycle
	alerted := make(map[string]bool)

	for _, alert := range result.Alerts {
		key := alertKey(alert.Dimension, alert.Metric)
		alerted[key] = true
		am.normalCounters[key] = 0

		// If already active for this key, skip (don't duplicate)
		if _, exists := am.active[key]; exists {
			continue
		}

		// New alert
		a := alert // copy
		am.active[key] = &a
		log.Printf("alert [%s] %s: %s", a.Severity, a.Dimension, a.Message)

		// Fire webhook
		if am.webhook != nil {
			am.webhook.Send(webhook.Event{
				EventType: "anomaly_" + string(a.Severity),
				Action:    string(a.Severity),
				TenantID:  a.Dimension,
				Model:     a.Metric,
				Message:   a.Message,
			})
		}
	}

	// Check for auto-resolve: if a previously alerted key wasn't alerted this cycle
	for key, alert := range am.active {
		if !alerted[key] {
			am.normalCounters[key]++
			if am.normalCounters[key] >= am.resolveAfter {
				now := time.Now()
				alert.State = "resolved"
				alert.ResolvedAt = &now
				am.history = append(am.history, alert)
				if len(am.history) > am.maxHistory {
					am.history = am.history[1:]
				}
				delete(am.active, key)
				delete(am.normalCounters, key)
				log.Printf("alert resolved: %s %s", alert.Dimension, alert.Metric)
			}
		}
	}
}

func (am *AlertManager) ActiveAlerts() []*Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()
	result := make([]*Alert, 0, len(am.active))
	for _, a := range am.active {
		result = append(result, a)
	}
	return result
}

func (am *AlertManager) RecentAlerts(limit int) []*Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()
	all := make([]*Alert, 0, len(am.active)+len(am.history))
	for _, a := range am.active {
		all = append(all, a)
	}
	// Add history in reverse (newest first)
	for i := len(am.history) - 1; i >= 0; i-- {
		all = append(all, am.history[i])
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

func (am *AlertManager) Acknowledge(id string) bool {
	am.mu.Lock()
	defer am.mu.Unlock()
	for _, a := range am.active {
		if a.ID == id {
			a.State = "acknowledged"
			return true
		}
	}
	return false
}
