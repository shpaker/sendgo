package usecase

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// StreamDownload serves GET /api/download/:id (and the /blob/:id variant).
//  1. Opens the blob and passes a ReadCloser to the handler for copying into
//     http.ResponseWriter.
//  2. After the copy completes, Finalize is invoked: counter increment; if the
//     limit is reached, remove blob + meta + cleanup entry.
//
// The two-phase API is required because Node increments the counter only
// after a successful delivery. We honor the same contract: the handler
// streams, then calls Finalize.
type StreamDownload struct {
	Blob port.BlobStorage
	Meta port.MetaStore
}

// Open opens the blob under the key the HMAC middleware already resolved.
// Returns a ReadCloser; the handler is responsible for io.Copy into the
// response.
func (uc *StreamDownload) Open(ctx context.Context, share *domain.FileShare) (io.ReadCloser, error) {
	key := blobKey(share)
	rc, err := uc.Blob.Get(ctx, key)
	if errors.Is(err, domain.ErrNotFound) {
		// Blob is already gone (parallel cleanup or a prior download removed it).
		_ = uc.Meta.Delete(ctx, share.ID)
		_ = uc.Meta.RemoveCleanup(ctx, key)
		return nil, domain.ErrNotFound
	}
	return rc, err
}

// Finalize increments the download counter; if the limit is reached, removes
// blob+meta. Must be called even after a failed stream (the handler decides),
// since the Node server incremented in `download.js` after a successful pipe.
func (uc *StreamDownload) Finalize(ctx context.Context, share *domain.FileShare) error {
	dl, err := uc.Meta.IncrField(ctx, share.ID, "dl")
	if err != nil {
		return fmt.Errorf("dl incr: %w", err)
	}
	if int(dl) >= share.DLimit {
		key := blobKey(share)
		_ = uc.Blob.Delete(ctx, key)
		_ = uc.Meta.RemoveCleanup(ctx, key)
		_ = uc.Meta.Delete(ctx, share.ID)
	}
	return nil
}

func blobKey(share *domain.FileShare) string {
	return fmt.Sprintf("%d-%s", share.Prefix, share.ID)
}
