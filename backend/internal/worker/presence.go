// Package worker holds background jobs that run on a schedule rather than on a request.
package worker

import (
	"context"
	"log/slog"
	"time"
)

type deviceSweeper interface {
	MarkStaleDevicesOffline(ctx context.Context, silentFor time.Duration) (int, error)
}

type loginSweeper interface {
	SweepLogins(ctx context.Context) (int, error)
}

// Presence marks devices offline once they stop sending heartbeats, and clears out
// expired logins on the same tick rather than starting a second goroutine for it.
type Presence struct {
	devices  deviceSweeper
	logins   loginSweeper
	log      *slog.Logger
	interval time.Duration
	timeout  time.Duration
}

func NewPresence(devices deviceSweeper, logins loginSweeper, log *slog.Logger, interval, timeout time.Duration) *Presence {
	return &Presence{
		devices:  devices,
		logins:   logins,
		log:      log,
		interval: interval,
		timeout:  timeout,
	}
}

// Run blocks until ctx is cancelled, so the caller decides its lifetime.
func (p *Presence) Run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	p.log.Info("presence watcher started", "interval", p.interval.String(), "timeout", p.timeout.String())

	for {
		select {
		case <-ctx.Done():
			p.log.Info("presence watcher stopped")
			return

		case <-ticker.C:
			p.sweep(ctx)
		}
	}
}

func (p *Presence) sweep(ctx context.Context) {
	marked, err := p.devices.MarkStaleDevicesOffline(ctx, p.timeout)
	if err != nil {
		p.log.Error("presence sweep failed", "error", err)
	} else if marked > 0 {
		p.log.Info("devices marked offline", "count", marked)
	}

	// A dead device and an expired token are unrelated, so one failing must not
	// stop the other from being cleaned up.
	if p.logins == nil {
		return
	}

	gone, err := p.logins.SweepLogins(ctx)
	if err != nil {
		p.log.Error("login sweep failed", "error", err)
		return
	}

	if gone > 0 {
		p.log.Info("expired logins cleared", "count", gone)
	}
}
