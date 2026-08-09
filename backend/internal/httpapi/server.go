// Package httpapi is the HTTP surface of the Deskbridge server.
package httpapi

import (
	"log/slog"
	"net/http"
	"time"
)

// Server carries the dependencies that handlers need.
type Server struct {
	log       *slog.Logger
	version   string
	startedAt time.Time
}

// NewServer builds the HTTP layer.
func NewServer(log *slog.Logger, version string, startedAt time.Time) *Server {
	return &Server{
		log:       log,
		version:   version,
		startedAt: startedAt,
	}
}

// Routes returns the fully wired handler, middleware included.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/status", s.handleStatus)

	return s.withRequestLogging(mux)
}
