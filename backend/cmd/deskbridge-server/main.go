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
	"sync"
	"syscall"
	"time"

	"github.com/sathvik458/deskbridge/backend/internal/config"
	"github.com/sathvik458/deskbridge/backend/internal/database"
	"github.com/sathvik458/deskbridge/backend/internal/httpapi"
	"github.com/sathvik458/deskbridge/backend/internal/store"
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

	api := httpapi.NewServer(log, version, startedAt, dataStore, dataStore, dataStore, cfg.AllowedOrigin)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	presence := worker.NewPresence(dataStore, log, cfg.SweepInterval, cfg.DeviceTimeout)

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

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown timed out after %s: %w", cfg.ShutdownGrace, err)
	}

	background.Wait()

	log.Info("server stopped cleanly", "uptime", time.Since(startedAt).Round(time.Second).String())
	return nil
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
