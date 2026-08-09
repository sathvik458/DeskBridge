package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DESKBRIDGE_ADDR", "")
	t.Setenv("DESKBRIDGE_LOG_LEVEL", "")
	t.Setenv("DESKBRIDGE_SHUTDOWN_GRACE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}

	if cfg.ListenAddr != defaultListenAddr {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, defaultListenAddr)
	}
	if cfg.LogLevel != defaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, defaultLogLevel)
	}
	if cfg.ShutdownGrace != defaultShutdownGrace {
		t.Errorf("ShutdownGrace = %s, want %s", cfg.ShutdownGrace, defaultShutdownGrace)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv("DESKBRIDGE_ADDR", "0.0.0.0:9000")
	t.Setenv("DESKBRIDGE_LOG_LEVEL", "debug")
	t.Setenv("DESKBRIDGE_SHUTDOWN_GRACE", "45s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}

	if cfg.ListenAddr != "0.0.0.0:9000" {
		t.Errorf("ListenAddr = %q, want %q", cfg.ListenAddr, "0.0.0.0:9000")
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.ShutdownGrace != 45*time.Second {
		t.Errorf("ShutdownGrace = %s, want %s", cfg.ShutdownGrace, 45*time.Second)
	}
}

func TestLoadRejectsBadInput(t *testing.T) {
	tests := []struct {
		name        string
		addr        string
		level       string
		grace       string
		wantErrPart string
	}{
		{name: "unknown log level", level: "verbose", wantErrPart: "invalid log level"},
		{name: "unparseable duration", grace: "ten seconds", wantErrPart: "not a valid duration"},
		{name: "negative grace period", grace: "-5s", wantErrPart: "must be positive"},
		{name: "zero grace period", grace: "0s", wantErrPart: "must be positive"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DESKBRIDGE_ADDR", tc.addr)
			t.Setenv("DESKBRIDGE_LOG_LEVEL", tc.level)
			t.Setenv("DESKBRIDGE_SHUTDOWN_GRACE", tc.grace)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() succeeded, want an error")
			}

			if !strings.Contains(err.Error(), tc.wantErrPart) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantErrPart)
			}
		})
	}
}

func TestEmptyValueIsTreatedAsUnset(t *testing.T) {
	t.Setenv("DESKBRIDGE_ADDR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}

	if cfg.ListenAddr != defaultListenAddr {
		t.Errorf("ListenAddr = %q, want the default %q", cfg.ListenAddr, defaultListenAddr)
	}
}
