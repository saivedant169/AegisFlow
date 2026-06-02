package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aegisflow_requests_total",
		Help: "Total number of requests by tenant, path, and status",
	}, []string{"tenant", "method", "path", "status"})

	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "aegisflow_request_duration_seconds",
		Help:    "Request duration in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"tenant", "method", "path"})

	// policyDecisionsTotal counts governance decisions. The tool label is
	// intentionally omitted to keep cardinality bounded (tool names are
	// user-supplied); the tool is emitted on a log line instead.
	policyDecisionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aegisflow_policy_decisions_total",
		Help: "Total policy decisions by decision and protocol",
	}, []string{"decision", "protocol"})
)

// RecordPolicyDecision increments the policy-decision counter. Safe to call
// from any policy evaluation path (MCP gateway, direct handler).
func RecordPolicyDecision(decision, protocol string) {
	if decision == "" {
		decision = "unknown"
	}
	if protocol == "" {
		protocol = "unknown"
	}
	policyDecisionsTotal.WithLabelValues(decision, protocol).Inc()
}

func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		tenant := TenantFromContext(r.Context())
		tenantID := ""
		if tenant != nil {
			tenantID = tenant.ID
		}

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(rw.statusCode)

		requestsTotal.WithLabelValues(tenantID, r.Method, r.URL.Path, status).Inc()
		requestDuration.WithLabelValues(tenantID, r.Method, r.URL.Path).Observe(duration)
	})
}
