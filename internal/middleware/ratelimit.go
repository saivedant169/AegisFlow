package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/aegisflow/aegisflow/internal/ratelimit"
	"github.com/aegisflow/aegisflow/pkg/types"
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
				next.ServeHTTP(w, r)
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
