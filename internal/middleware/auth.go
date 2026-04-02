package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

type contextKey string

const TenantContextKey contextKey = "tenant"
const RoleContextKey contextKey = "role"

func TenantFromContext(ctx context.Context) *config.TenantConfig {
	t, _ := ctx.Value(TenantContextKey).(*config.TenantConfig)
	return t
}

func RoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(RoleContextKey).(string)
	return role
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

			match := cfg.FindTenantByAPIKey(apiKey)
			if match == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(types.NewErrorResponse(401, "authentication_error", "invalid API key"))
				return
			}

			ctx := context.WithValue(r.Context(), TenantContextKey, match.Tenant)
			ctx = context.WithValue(ctx, RoleContextKey, match.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SoftAuth is like Auth but doesn't reject missing API keys.
// If a key is present and valid, it sets tenant+role in context.
// If no key is present, the request continues without context (RBAC will handle the 403).
func SoftAuth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := extractAPIKey(r)
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			match := cfg.FindTenantByAPIKey(apiKey)
			if match == nil {
				// Invalid key — still proceed but without context
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), TenantContextKey, match.Tenant)
			ctx = context.WithValue(ctx, RoleContextKey, match.Role)
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
