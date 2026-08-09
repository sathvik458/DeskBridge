package httpapi

import (
	"net/http"
	"time"
)

// withRequestLogging logs one line per request, after it completes.
func (s *Server) withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.written,
			"duration_us", time.Since(started).Microseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

// statusRecorder wraps a ResponseWriter to remember what was sent.
type statusRecorder struct {
	http.ResponseWriter

	status  int
	written int
}

func (rec *statusRecorder) WriteHeader(code int) {
	rec.status = code
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *statusRecorder) Write(b []byte) (int, error) {
	n, err := rec.ResponseWriter.Write(b)
	rec.written += n
	return n, err
}
