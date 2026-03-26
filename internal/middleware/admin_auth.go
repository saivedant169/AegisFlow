package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/aegisflow/aegisflow/pkg/types"
)

// AdminAuth protects admin endpoints with a bearer token.
// If adminToken is empty, admin auth is disabled (open access).
func AdminAuth(adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth if no token configured
			if adminToken == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Skip auth for health endpoint
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			// Skip auth for dashboard (served publicly, data endpoints protected)
			if r.URL.Path == "/" || r.URL.Path == "/dashboard" {
				next.ServeHTTP(w, r)
				return
			}

			// Check Authorization header
			token := extractBearerToken(r)
			if token == "" {
				token = r.URL.Query().Get("token")
			}

			if subtle.ConstantTimeCompare([]byte(token), []byte(adminToken)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(types.NewErrorResponse(401, "authentication_error", "invalid or missing admin token"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}
