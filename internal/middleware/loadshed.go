package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/saivedant169/AegisFlow/internal/loadshed"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

// LoadShed returns middleware that enforces concurrency limits with
// priority-based admission control. Requests that cannot be admitted
// receive a 503 Service Unavailable response.
//
// Priority is determined by the X-Priority header ("high", "normal", "low").
// If no header is set, requests default to "normal" priority.
func LoadShed(shedder *loadshed.Shedder) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip load shedding for health checks.
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			priority := loadshed.ParsePriority(r.Header.Get("X-Priority"))

			result, release := shedder.Acquire(r.Context(), priority)
			switch result {
			case loadshed.Admitted:
				defer release()
				next.ServeHTTP(w, r)
			case loadshed.Shed:
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "5")
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(types.NewErrorResponse(
					503, "service_unavailable",
					"server at capacity -- request shed, retry later",
				))
			case loadshed.QueueTimeout:
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "5")
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(types.NewErrorResponse(
					503, "service_unavailable",
					"request queued too long -- timed out, retry later",
				))
			}
		})
	}
}
