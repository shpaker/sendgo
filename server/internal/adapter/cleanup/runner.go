// Package cleanup is an in-process goroutine that deletes blobs of shares
// whose TTL has expired.
//
// This fixes a bug in the current Node version: there, Redis EXPIRE drops the
// metadata but the blob in S3/FS is left orphaned. We keep our own cleanup
// index (sorted-set / sorted-slice depending on the MetaStore) and
// periodically call blob.Delete for each entry whose expiresAt has passed.
//
// Activated via --cleanup-enabled. When disabled, the goroutine does not
// start.
package cleanup

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// Runner is the configuration for the in-process garbage collector.
type Runner struct {
	Meta     port.MetaStore
	Blob     port.BlobStorage
	Clock    port.Clock
	Log      *slog.Logger
	Interval time.Duration
	Batch    int
}

// Run blocks on a ticker until Done(ctx). Must be launched in its own
// goroutine from main(): `go runner.Run(ctx)`.
func (r *Runner) Run(ctx context.Context) {
	if r.Interval <= 0 {
		r.Log.Warn("cleanup: interval <= 0, runner disabled")
		return
	}
	t := time.NewTicker(r.Interval)
	defer t.Stop()
	r.Log.Info("cleanup runner started", "interval", r.Interval, "batch", r.Batch)
	for {
		select {
		case <-ctx.Done():
			r.Log.Info("cleanup runner stopped")
			return
		case <-t.C:
			r.tick(ctx)
		}
	}
}

func (r *Runner) tick(ctx context.Context) {
	keys, err := r.Meta.DequeueCleanup(ctx, r.Clock.Now(), r.Batch)
	if err != nil {
		r.Log.Error("cleanup dequeue failed", "err", err)
		return
	}
	if len(keys) == 0 {
		return
	}
	deleted := 0
	for _, k := range keys {
		if err := r.Blob.Delete(ctx, k); err != nil && !errors.Is(err, domain.ErrNotFound) {
			r.Log.Error("cleanup blob delete failed", "key", k, "err", err)
			continue
		}
		if err := r.Meta.RemoveCleanup(ctx, k); err != nil {
			r.Log.Error("cleanup index remove failed", "key", k, "err", err)
			continue
		}
		deleted++
	}
	r.Log.Info("cleanup tick", "candidates", len(keys), "deleted", deleted)
}
