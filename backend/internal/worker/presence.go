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

// Presence marks devices offline once they stop sending heartbeats.
type Presence struct {
	devices  deviceSweeper
	log      *slog.Logger
	interval time.Duration
	timeout  time.Duration
}

func NewPresence(devices deviceSweeper, log *slog.Logger, interval, timeout time.Duration) *Presence {
	return &Presence{
		devices:  devices,
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
		return
	}

	if marked > 0 {
		p.log.Info("devices marked offline", "count", marked)
	}
}
