package port

import (
	"context"
	"io"
)

// BlobStorage is the abstraction over the encrypted-file store.
// Implementations: S3 (aws-sdk-go-v2) and FS (local filesystem).
// The key is a "{day_prefix}-{share_id}" string, matching the current Node code.
type BlobStorage interface {
	// Put streams r into the store under key. expireSeconds is provided for
	// backends with native TTL (S3 lifecycle handles this via tags/prefixes);
	// the FS implementation ignores it — TTL lives in the meta store.
	Put(ctx context.Context, key string, r io.Reader, expireSeconds int) error

	// Get returns the ciphertext stream. The caller must close the ReadCloser.
	// If the object is missing, domain.ErrNotFound is returned.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Head is a lightweight existence + size check. It does not open the body.
	Head(ctx context.Context, key string) (size int64, exists bool, err error)

	// Delete removes the object; if it is already gone, returns nil (idempotent).
	Delete(ctx context.Context, key string) error

	// Length returns the size of an existing object; ErrNotFound otherwise.
	Length(ctx context.Context, key string) (int64, error)

	// Ping is the liveness probe for /__heartbeat__. A light operation
	// (e.g. HeadBucket for S3).
	Ping(ctx context.Context) error
}
