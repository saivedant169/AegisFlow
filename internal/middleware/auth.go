package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/aegisflow/aegisflow/internal/config"
	"github.com/aegisflow/aegisflow/pkg/types"
)

type contextKey string

const TenantContextKey contextKey = "tenant"

func TenantFromContext(ctx context.Context) *config.TenantConfig {
	t, _ := ctx.Value(TenantContextKey).(*config.TenantConfig)
	return t
}

func Auth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for health checks
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			apiKey := extractAPIKey(r)
			if apiKey == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(types.NewErrorResponse(401, "authentication_error", "missing API key — use X-API-Key header or Authorization: Bearer <key>"))
				return
			}

			tenant := cfg.FindTenantByAPIKey(apiKey)
			if tenant == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(types.NewErrorResponse(401, "authentication_error", "invalid API key"))
				return
			}

			ctx := context.WithValue(r.Context(), TenantContextKey, tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func extractAPIKey(r *http.Request) string {
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}

	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	return ""
}
