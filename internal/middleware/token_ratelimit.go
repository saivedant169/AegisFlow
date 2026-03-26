package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/aegisflow/aegisflow/internal/ratelimit"
	"github.com/aegisflow/aegisflow/pkg/types"
)

// TokenRateLimit enforces tokens-per-minute limits by estimating input tokens
// from the request body before forwarding to the provider.
func TokenRateLimit(limiter ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" || r.URL.Path != "/v1/chat/completions" {
				next.ServeHTTP(w, r)
				return
			}

			tenant := TenantFromContext(r.Context())
			if tenant == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Estimate tokens from content length (rough: len/4)
			estimatedTokens := 0
			if r.ContentLength > 0 {
				estimatedTokens = int(r.ContentLength) / 4
			}
			if estimatedTokens < 1 {
				estimatedTokens = 1
			}

			allowed, err := limiter.Allow("tok:"+tenant.ID, estimatedTokens)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			if !allowed {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(types.NewErrorResponse(429, "rate_limit_error", "token rate limit exceeded — retry after 60 seconds"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
