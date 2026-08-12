// Package config loads Deskbridge server configuration from the environment.
package config

import (
	"fmt"
	"os"
	"time"
)

const (
	defaultListenAddr    = "localhost:8080"
	defaultLogLevel      = "info"
	defaultShutdownGrace = 10 * time.Second
	defaultDatabasePath  = "deskbridge.db"
	defaultBusyTimeout   = 5 * time.Second
)

// Config holds everything the server needs to know at startup.
type Config struct {
	ListenAddr    string
	LogLevel      string
	ShutdownGrace time.Duration
	DatabasePath  string
	BusyTimeout   time.Duration
}

// Load reads configuration from the environment, applies defaults and validates.
func Load() (Config, error) {
	grace, err := envDuration("DESKBRIDGE_SHUTDOWN_GRACE", defaultShutdownGrace)
	if err != nil {
		return Config{}, err
	}

	busyTimeout, err := envDuration("DESKBRIDGE_BUSY_TIMEOUT", defaultBusyTimeout)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		ListenAddr:    envString("DESKBRIDGE_ADDR", defaultListenAddr),
		LogLevel:      envString("DESKBRIDGE_LOG_LEVEL", defaultLogLevel),
		ShutdownGrace: grace,
		DatabasePath:  envString("DESKBRIDGE_DB_PATH", defaultDatabasePath),
		BusyTimeout:   busyTimeout,
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("listen address must not be empty")
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log level %q: want debug, info, warn or error", c.LogLevel)
	}

	if c.ShutdownGrace <= 0 {
		return fmt.Errorf("shutdown grace must be positive, got %s", c.ShutdownGrace)
	}

	if c.DatabasePath == "" {
		return fmt.Errorf("database path must not be empty")
	}

	if c.BusyTimeout <= 0 {
		return fmt.Errorf("busy timeout must be positive, got %s", c.BusyTimeout)
	}

	return nil
}

// envString returns the value of key, treating empty the same as unset.
func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a valid duration: %w", key, raw, err)
	}

	return d, nil
}
