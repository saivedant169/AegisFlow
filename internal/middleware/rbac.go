package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/saivedant169/AegisFlow/pkg/types"
)

var roleHierarchy = map[string]int{
	"viewer":   1,
	"operator": 2,
	"admin":    3,
}

// RBAC returns middleware that requires the caller's role to be at or above requiredRole.
// Role hierarchy: admin > operator > viewer.
func RBAC(requiredRole string) func(http.Handler) http.Handler {
	requiredLevel := roleHierarchy[requiredRole]
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := RoleFromContext(r.Context())
			if role == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(types.NewErrorResponse(403, "forbidden", "authentication required"))
				return
			}

			userLevel := roleHierarchy[role]
			if userLevel < requiredLevel {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(types.NewErrorResponse(403, "forbidden", "insufficient permissions — requires "+requiredRole+" role"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
