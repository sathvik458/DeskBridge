package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const heartbeatEvery = 25 * time.Second

func (s *Server) handleLiveStream(w http.ResponseWriter, r *http.Request) {
	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		s.writeError(w, http.StatusInternalServerError, "this server cannot stream")
		return
	}

	// The server sets a 15s WriteTimeout, which would cut a stream that is meant to
	// stay open for hours. ResponseController clears the deadline for this one
	// connection without weakening it for ordinary requests.
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		s.log.Error("clearing the write deadline", "error", err)
		s.writeError(w, http.StatusInternalServerError, "this server cannot stream")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	mail, stopWatching := s.feed.Watch()
	defer stopWatching()

	s.log.Info("stream opened", "watching", s.feed.Watching(), "remote", r.RemoteAddr)
	defer func() { s.log.Info("stream closed", "remote", r.RemoteAddr) }()

	pulse := time.NewTicker(heartbeatEvery)
	defer pulse.Stop()

	for {
		select {
		case <-r.Context().Done():
			return

		case event, open := <-mail:
			if !open {
				return
			}

			payload, err := json.Marshal(event)
			if err != nil {
				s.log.Error("encoding event", "kind", event.Kind, "error", err)
				continue
			}

			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Kind, payload); err != nil {
				return
			}
			flusher.Flush()

		case <-pulse.C:
			// A comment line keeps proxies and load balancers from closing an idle
			// connection, and costs three bytes.
			if _, err := fmt.Fprint(w, ": still here\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
