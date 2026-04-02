package middleware

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/saivedant169/AegisFlow/internal/ratelimit"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

func RateLimit(limiter ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			tenant := TenantFromContext(r.Context())
			if tenant == nil {
				next.ServeHTTP(w, r)
				return
			}

			allowed, err := limiter.Allow(tenant.ID, 1)
			if err != nil {
				log.Printf("rate limiter error (denying request): %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(types.NewErrorResponse(503, "service_error", "rate limiter unavailable — try again later"))
				return
			}

			if !allowed {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(types.NewErrorResponse(429, "rate_limit_error", "rate limit exceeded — retry after 60 seconds"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
