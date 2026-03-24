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
)

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
