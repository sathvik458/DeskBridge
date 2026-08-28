package httpapi

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

type healthResponse struct {
	Status string `json:"status"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

type statusResponse struct {
	Status        string `json:"status"`
	Version       string `json:"version"`
	StartedAt     string `json:"started_at"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	Uptime        string `json:"uptime"`
	GoVersion     string `json:"go_version"`
	Platform      string `json:"platform"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(s.startedAt)

	s.writeJSON(w, http.StatusOK, statusResponse{
		Status:        "ok",
		Version:       s.version,
		StartedAt:     s.startedAt.UTC().Format(time.RFC3339),
		UptimeSeconds: int64(uptime.Seconds()),
		Uptime:        uptime.Round(time.Second).String(),
		GoVersion:     runtime.Version(),
		Platform:      runtime.GOOS + "/" + runtime.GOARCH,
	})
}

// Order matters: headers, status, then body.
func (s *Server) writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(body); err != nil {
		s.log.Error("writing response body", "error", err)
	}
}
