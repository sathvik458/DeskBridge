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

type fileIndex interface {
	StoredPaths(ctx context.Context) (map[string]bool, error)
}

type blobStore interface {
	Stored() (map[string]time.Time, error)
	Discard(relative string) error
}

type bucketSweeper interface {
	SweepLimiters() int
}

// blobGrace is how long a file has to be parentless before it counts as orphaned.
const blobGrace = time.Hour

// Presence marks devices offline once they stop sending heartbeats, and clears out
// expired logins on the same tick rather than starting a second goroutine for it.
type Presence struct {
	devices  deviceSweeper
	logins   loginSweeper
	files    fileIndex
	blobs    blobStore
	buckets  bucketSweeper
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

// AlsoSweep wires in the jobs that arrived later. They are optional so the worker
// can still be built with nothing but a device sweeper in a test.
func (p *Presence) AlsoSweep(files fileIndex, blobs blobStore, buckets bucketSweeper) {
	p.files = files
	p.blobs = blobs
	p.buckets = buckets
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

// Each job is independent, so one failing or being unwired must never stop the
// rest of the tick. Everything here either logs and moves on or returns early
// from its own function, never from this one.
func (p *Presence) sweep(ctx context.Context) {
	p.sweepDevices(ctx)
	p.sweepLogins(ctx)
	p.sweepBlobs(ctx)

	if p.buckets != nil {
		p.buckets.SweepLimiters()
	}
}

func (p *Presence) sweepDevices(ctx context.Context) {
	marked, err := p.devices.MarkStaleDevicesOffline(ctx, p.timeout)
	if err != nil {
		p.log.Error("presence sweep failed", "error", err)
		return
	}

	if marked > 0 {
		p.log.Info("devices marked offline", "count", marked)
	}
}

func (p *Presence) sweepLogins(ctx context.Context) {
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

// A blob with no row is what an upload that failed halfway leaves behind. The age
// check is the important part: a file written seconds ago may belong to a request
// that has not written its row yet, and deleting it would break a live upload.
func (p *Presence) sweepBlobs(ctx context.Context) {
	if p.files == nil || p.blobs == nil {
		return
	}

	known, err := p.files.StoredPaths(ctx)
	if err != nil {
		p.log.Error("reading stored paths", "error", err)
		return
	}

	onDisk, err := p.blobs.Stored()
	if err != nil {
		p.log.Error("walking the vault", "error", err)
		return
	}

	cutoff := time.Now().Add(-blobGrace)
	removed := 0

	for path, written := range onDisk {
		if known[path] || written.After(cutoff) {
			continue
		}

		if err := p.blobs.Discard(path); err != nil {
			p.log.Error("removing an orphaned blob", "path", path, "error", err)
			continue
		}

		removed++
	}

	if removed > 0 {
		p.log.Info("orphaned files removed", "count", removed)
	}
}
