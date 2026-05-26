package usecase

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// StreamUpload is the second phase of the WS upload: it proxies bytes from
// the client to BlobStorage with a size cap (encrypted size), then writes
// meta to the store + cleanup index.
//
// If anything fails mid-way, we try to remove the partially uploaded blob to
// preserve the invariant: no meta without a blob and no blob without meta.
type StreamUpload struct {
	Blob    port.BlobStorage
	Meta    port.MetaStore
	MaxSize int64 // encrypted size (post-EncryptedSize())
	Clock   port.Clock
}

// Execute streams r into the blob under s3Key; on success it persists the
// share + cleanup entry. Returns the actual number of bytes written.
func (uc *StreamUpload) Execute(ctx context.Context, share *domain.FileShare, s3Key string, r io.Reader) (int64, error) {
	// counting + limited reader: precise error on limit overrun, without fully
	// buffering in memory.
	cr := &countingReader{r: io.LimitReader(r, uc.MaxSize+1)}
	if err := uc.Blob.Put(ctx, s3Key, cr, share.ExpireSeconds); err != nil {
		return cr.n, fmt.Errorf("blob put: %w", err)
	}
	if cr.n > uc.MaxSize {
		// LimitReader let one byte past the threshold — delete the blob right away.
		_ = uc.Blob.Delete(ctx, s3Key)
		return cr.n, domain.ErrPayloadTooLarge
	}
	share.SizeBytes = cr.n

	// Blob written — now meta.
	if err := uc.Meta.Set(ctx, share, share.ExpireSeconds); err != nil {
		// Roll back the blob, otherwise we'd leak an orphan.
		_ = uc.Blob.Delete(ctx, s3Key)
		return cr.n, fmt.Errorf("meta set: %w", err)
	}
	expiresAt := uc.Clock.Now().Add(time.Duration(share.ExpireSeconds) * time.Second)
	if err := uc.Meta.EnqueueCleanup(ctx, s3Key, expiresAt); err != nil {
		// Non-fatal — meta is already in place; cleanup just won't run for this
		// share. Logged at the adapter level; the use case continues.
		_ = err
	}
	return cr.n, nil
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
