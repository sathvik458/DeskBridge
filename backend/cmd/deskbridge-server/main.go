// Command deskbridge-server is the Deskbridge backend.
//
// It currently serves a single endpoint, /health, so I can tell from my Mac
// whether the Bahrain machine is actually up. Everything else gets added phase
// by phase.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/config"
)

// version is overwritten at build time:
//
//	go build -ldflags "-X main.version=$(git describe --always)" ./cmd/deskbridge-server
var version = "dev"

// main stays deliberately tiny. All the real work happens in run, which
// returns an error instead of exiting, so that deferred cleanup actually runs
// and so the startup path can be tested later. os.Exit skips defers.
func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "deskbridge: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	log := newLogger(cfg.LogLevel)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth(log))

	// Building the http.Server by hand rather than calling http.ListenAndServe,
	// because the convenience function gives you no timeouts at all. A server
	// with no timeouts will happily hold a connection open forever, which is
	// how a slow or dead client leaks a goroutine on a machine I cannot reach.
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// ctx is cancelled the first time this process receives an interrupt or a
	// termination signal. The second such signal kills the process outright,
	// which is the behaviour I want: one Ctrl+C asks politely, two insists.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ListenAndServe blocks until the server stops, so it runs on its own
	// goroutine and reports back over a buffered channel. The buffer matters:
	// if shutdown wins the race below, nothing is left to receive from this
	// channel, and an unbuffered send would block the goroutine forever.
	serverErr := make(chan error, 1)

	go func() {
		log.Info("server starting", "addr", cfg.ListenAddr, "version", version)

		// Shutdown makes ListenAndServe return ErrServerClosed. That is the
		// expected ending, not a failure, so it is filtered out here.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}

		serverErr <- nil
	}()

	// Whichever happens first wins: the server dies on its own, or a signal
	// arrives asking it to stop.
	select {
	case err := <-serverErr:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil

	case <-ctx.Done():
		log.Info("shutdown signal received", "grace", cfg.ShutdownGrace.String())
	}

	// A fresh context here, not ctx. ctx is already cancelled - that is why we
	// are shutting down - and passing a cancelled context to Shutdown would
	// abort every in-flight request immediately, which is the opposite of
	// graceful.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown timed out after %s: %w", cfg.ShutdownGrace, err)
	}

	log.Info("server stopped cleanly")
	return nil
}

// newLogger builds the one logger the program uses.
//
// There is no logging package yet, because only main constructs a logger and
// everything else receives one. A package would be ceremony without a caller.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level

	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	// TextHandler while I am reading logs by eye in a terminal. This becomes
	// JSON once the logs are written to a file on the Bahrain PC and I am
	// grepping them from India.
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})

	return slog.New(handler)
}

// healthResponse is the body of GET /health.
//
// A named struct rather than a map, so the shape of the response is visible in
// the code and a mistyped JSON key becomes a compile error instead of a silent
// change to the API.
type healthResponse struct {
	Status string `json:"status"`
}

// handleHealth returns the handler for GET /health.
//
// It is a function that returns a http.HandlerFunc rather than a plain handler
// so the logger can be passed in. Handlers get their dependencies as
// arguments; nothing here reaches for a package-level global.
func handleHealth(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(healthResponse{Status: "ok"}); err != nil {
			// The status line has already been sent by this point, so there is
			// no way to tell the client. Logging it is the only honest option.
			log.Error("writing health response", "error", err)
		}
	}
}
