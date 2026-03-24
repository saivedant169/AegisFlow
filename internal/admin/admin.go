package admin

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/aegisflow/aegisflow/internal/usage"
)

type Server struct {
	tracker *usage.Tracker
}

func NewServer(tracker *usage.Tracker) *Server {
	return &Server{tracker: tracker}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)

	r.Get("/health", s.healthHandler)
	r.Get("/metrics", promhttp.Handler().ServeHTTP)
	r.Get("/admin/v1/usage", s.usageHandler)

	return r
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) usageHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.tracker.GetAllUsage())
}
