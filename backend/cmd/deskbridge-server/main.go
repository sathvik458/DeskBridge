// Command deskbridge-server is the Deskbridge backend.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/config"
	"github.com/sathvik458/deskbridge/backend/internal/database"
	"github.com/sathvik458/deskbridge/backend/internal/httpapi"
	"github.com/sathvik458/deskbridge/backend/internal/live"
	"github.com/sathvik458/deskbridge/backend/internal/store"
	"github.com/sathvik458/deskbridge/backend/internal/vault"
	"github.com/sathvik458/deskbridge/backend/internal/worker"
)

// version is overwritten at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "deskbridge: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	startedAt := time.Now()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	log := newLogger(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DatabasePath, cfg.BusyTimeout)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()

	log.Info("database opened", "path", cfg.DatabasePath)

	if err := database.Migrate(ctx, db, log); err != nil {
		return fmt.Errorf("migrating database: %w", err)
	}

	dataStore := store.New(db)

	shared, err := vault.Open(cfg.FilesPath)
	if err != nil {
		return fmt.Errorf("opening the file vault: %w", err)
	}

	log.Info("file vault ready", "path", shared.Root())

	feed := live.NewFeed(log)

	api := httpapi.NewServer(log, version, startedAt, dataStore, dataStore, dataStore, dataStore,
		dataStore, dataStore, dataStore, dataStore, dataStore, dataStore, dataStore, shared, feed,
		cfg.AllowedOrigin, cfg.SecureCookies)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           withFrontend(api.Routes(), cfg.WebPath, log),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Shutdown waits for every handler to return, and a live stream only returns
	// when its feed closes.
	srv.RegisterOnShutdown(feed.Close)

	presence := worker.NewPresence(dataStore, dataStore, log, cfg.SweepInterval, cfg.DeviceTimeout)
	presence.AlsoSweep(dataStore, shared, api, dataStore)

	var background sync.WaitGroup
	background.Add(1)

	go func() {
		defer background.Done()
		presence.Run(ctx)
	}()

	serverErr := make(chan error, 1)

	go func() {
		log.Info("server starting",
			"addr", cfg.ListenAddr,
			"version", version,
			"log_level", cfg.LogLevel,
		)

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}

		serverErr <- nil
	}()

	select {
	case err := <-serverErr:
		stop()
		background.Wait()
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil

	case <-ctx.Done():
		log.Info("shutdown signal received", "grace", cfg.ShutdownGrace.String())
	}

	// Fresh context: ctx is already cancelled, which would abort in-flight work.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancel()

	shutErr := srv.Shutdown(shutdownCtx)

	// Waited for on both paths, so the database is never closed under a sweep that
	// is still running.
	background.Wait()

	if shutErr != nil {
		return fmt.Errorf("graceful shutdown timed out after %s: %w", cfg.ShutdownGrace, shutErr)
	}

	log.Info("server stopped cleanly", "uptime", time.Since(startedAt).Round(time.Second).String())
	return nil
}

// withFrontend serves the built dashboard from the same address as the API, so a
// browser needs one URL and nobody has to run the Vite dev server to use the app.
// Without a build it says so and carries on, because the API alone is still useful.
func withFrontend(api http.Handler, dir string, log *slog.Logger) http.Handler {
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		log.Warn("no built frontend found, serving the API only", "path", dir)
		return api
	}

	root := http.NewServeMux()
	root.Handle("/api/", api)
	root.Handle("/health", api)
	root.Handle("/", http.FileServer(http.Dir(dir)))

	log.Info("serving the frontend", "path", dir)

	return root
}

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

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})

	return slog.New(handler)
}
