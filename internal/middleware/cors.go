package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/saivedant169/AegisFlow/internal/config"
)

func CORS(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.Server.CORS.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			allowed := false
			for _, o := range cfg.Server.CORS.AllowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				// Only set credentials header when origins are explicitly listed.
				// Credentials + wildcard origin is a security misconfiguration that
				// allows any website to make authenticated requests.
				if cfg.Server.CORS.AllowCredentials && !hasWildcardOrigin(cfg.Server.CORS.AllowedOrigins) {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				if len(cfg.Server.CORS.ExposedHeaders) > 0 {
					w.Header().Set("Access-Control-Expose-Headers", strings.Join(cfg.Server.CORS.ExposedHeaders, ", "))
				}

				if r.Method == http.MethodOptions {
					w.Header().Set("Access-Control-Allow-Methods", strings.Join(cfg.Server.CORS.AllowedMethods, ", "))
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(cfg.Server.CORS.AllowedHeaders, ", "))
					w.Header().Set("Access-Control-Max-Age", strconv.Itoa(cfg.Server.CORS.MaxAge))
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

func hasWildcardOrigin(origins []string) bool {
	for _, o := range origins {
		if o == "*" {
			return true
		}
	}
	return false
}
