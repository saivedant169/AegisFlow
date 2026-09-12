package middleware

import (
	"log"

	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/saivedant169/AegisFlow/internal/config"
	"github.com/saivedant169/AegisFlow/pkg/types"
)

type contextKey string

const TenantContextKey contextKey = "tenant"
const RoleContextKey contextKey = "role"
const principalContextKey contextKey = "principal"

// PrincipalFromContext identifies the authenticated key without exposing it.
func PrincipalFromContext(ctx context.Context) string {
	principal, _ := ctx.Value(principalContextKey).(string)
	return principal
}

// ScopedSessionID binds a client session label to authenticated identity.
func ScopedSessionID(ctx context.Context, label string) string {
	tenant := TenantFromContext(ctx)
	if tenant == nil || PrincipalFromContext(ctx) == "" {
		return ""
	}
	data, _ := json.Marshal([]string{"session-v1", tenant.ID, PrincipalFromContext(ctx), label})
	return fmt.Sprintf("session-v1-%x", sha256.Sum256(data))
}

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
			if r.URL.Path == "/health" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
				next.ServeHTTP(w, r)
				return
			}

			apiKey := extractAPIKey(r)
			if apiKey == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				if err := json.NewEncoder(w).Encode(types.NewErrorResponse(401, "authentication_error", "missing API key: use X-API-Key header or Authorization: Bearer <key>")); err != nil {
					log.Print("JSON response write failed")
					return
				}
				return
			}

			match := cfg.FindTenantByAPIKey(apiKey)
			if match == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				if err := json.NewEncoder(w).Encode(types.NewErrorResponse(401, "authentication_error", "invalid API key")); err != nil {
					log.Print("JSON response write failed")
					return
				}
				return
			}

			ctx := context.WithValue(r.Context(), TenantContextKey, match.Tenant)
			ctx = context.WithValue(ctx, RoleContextKey, match.Role)
			data, _ := json.Marshal([]string{"principal-v1", match.Tenant.ID, apiKey})
			ctx = context.WithValue(ctx, principalContextKey, fmt.Sprintf("principal-v1-%x", sha256.Sum256(data)))
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
				// Invalid key: still proceed but without context
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), TenantContextKey, match.Tenant)
			ctx = context.WithValue(ctx, RoleContextKey, match.Role)
			data, _ := json.Marshal([]string{"principal-v1", match.Tenant.ID, apiKey})
			ctx = context.WithValue(ctx, principalContextKey, fmt.Sprintf("principal-v1-%x", sha256.Sum256(data)))
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
