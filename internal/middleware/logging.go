package middleware

import (
	"log"
	"net/http"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		tenant := TenantFromContext(r.Context())
		tenantID := ""
		if tenant != nil {
			tenantID = tenant.ID
		}

		log.Printf("method=%s path=%s status=%d duration=%s tenant=%s",
			r.Method, r.URL.Path, rw.statusCode, time.Since(start), tenantID)
	})
}
