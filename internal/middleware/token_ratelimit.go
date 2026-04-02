package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/saivedant169/AegisFlow/internal/ratelimit"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

// TokenRateLimit enforces tokens-per-minute limits by reading the actual
// request body size rather than trusting Content-Length (which can be 0
// for chunked transfers or spoofed by clients).
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

			// Read the actual body to get real size (don't trust Content-Length)
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				log.Printf("token rate limit: failed to read body: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(types.NewErrorResponse(400, "invalid_request", "failed to read request body"))
				return
			}
			// Put the body back so downstream handlers can read it
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			// Estimate tokens from actual body size (rough: len/4)
			estimatedTokens := len(bodyBytes) / 4
			if estimatedTokens < 1 {
				estimatedTokens = 1
			}

			allowed, err := limiter.Allow("tok:"+tenant.ID, estimatedTokens)
			if err != nil {
				log.Printf("token rate limiter error (denying request): %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				json.NewEncoder(w).Encode(types.NewErrorResponse(503, "service_error", "rate limiter unavailable — try again later"))
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
