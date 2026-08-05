// Package config loads Deskbridge server configuration from the environment.
//
// Configuration is read exactly once, at startup, by main. Nothing else in the
// program touches os.Getenv - every other package receives a Config value.
// That keeps the rest of the code testable: a test builds a Config literal
// instead of mutating global process state.
package config

import (
	"fmt"
	"os"
	"time"
)

// Defaults used when the matching environment variable is unset.
//
// The defaults are the values that make sense on my Mac while developing. The
// Bahrain PC overrides them through the environment.
const (
	defaultListenAddr    = "localhost:8080"
	defaultLogLevel      = "info"
	defaultShutdownGrace = 10 * time.Second
)

// Config holds everything the server needs to know at startup.
//
// It is passed around by value rather than as a pointer. The struct is small,
// and copying it makes it clear that nothing can reconfigure a running server
// by accident.
type Config struct {
	// ListenAddr is the host:port the HTTP server binds to. On the Bahrain PC
	// this becomes an address on the private overlay network, not localhost.
	ListenAddr string

	// LogLevel is one of debug, info, warn, error.
	LogLevel string

	// ShutdownGrace is how long to let in-flight requests finish before the
	// process gives up and exits anyway.
	ShutdownGrace time.Duration
}

// Load reads configuration from the environment, applies defaults, and
// validates the result.
//
// It returns an error instead of exiting. A package that calls os.Exit is a
// package that cannot be tested, and it steals a decision that belongs to
// main.
func Load() (Config, error) {
	grace, err := envDuration("DESKBRIDGE_SHUTDOWN_GRACE", defaultShutdownGrace)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		ListenAddr:    envString("DESKBRIDGE_ADDR", defaultListenAddr),
		LogLevel:      envString("DESKBRIDGE_LOG_LEVEL", defaultLogLevel),
		ShutdownGrace: grace,
	}

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// validate rejects a Config that would fail later in a confusing way.
//
// Failing here means the server refuses to start with a clear message, rather
// than starting and then misbehaving at 2am on a machine I cannot see.
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

	return nil
}

// envString returns the value of key, or fallback if it is unset or empty.
//
// An empty variable is treated the same as an unset one on purpose:
// DESKBRIDGE_ADDR="" is almost always a mistake in a shell script, not a
// deliberate request for an empty address.
func envString(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envDuration parses key as a Go duration string, for example "30s" or "2m".
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
