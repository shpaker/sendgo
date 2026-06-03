package port

import (
	"context"
	"time"

	"github.com/sendgo/sendgo/server/internal/domain"
)

// MetaStore is the abstraction over share metadata storage. Implementations:
// Redis (`server/internal/adapter/meta/redis_store.go`) and in-memory
// (`memory_store.go`). Share fields are stored as a Redis hash 1:1 with the
// current Node schema; the cleanup index is a separate sorted-set /
// sorted-slice structure added in the Go version to fix the orphaned-blob bug.
type MetaStore interface {
	// Set creates a share record with the given TTL.
	Set(ctx context.Context, share *domain.FileShare, expireSeconds int) error

	// Get returns the share by id. If it is missing or expired,
	// domain.ErrNotFound is returned.
	Get(ctx context.Context, id domain.ShareID) (*domain.FileShare, error)

	// SetField updates a single field (used for nonce, auth, pwd, dlimit).
	// Field names match the Redis hash: "nonce", "auth", "pwd", "dlimit", "dl".
	SetField(ctx context.Context, id domain.ShareID, field string, value string) error

	// IncrField atomically increments a numeric field (used for "dl"). Returns
	// the new value.
	IncrField(ctx context.Context, id domain.ShareID, field string) (int64, error)

	// Delete removes the share record and its associated cleanup entry
	// (idempotent).
	Delete(ctx context.Context, id domain.ShareID) error

	// TTL returns the remaining time-to-live for the record.
	TTL(ctx context.Context, id domain.ShareID) (time.Duration, error)

	// EnqueueCleanup adds an entry to the deletion index: "once expiresAt is
	// reached, the blob with this key can be safely deleted even if the meta
	// record has already expired".
	EnqueueCleanup(ctx context.Context, blobKey string, expiresAt time.Time) error

	// DequeueCleanup returns up to limit keys whose expiresAt ≤ before. The
	// method does not remove them immediately — the caller must explicitly
	// invoke RemoveCleanup after a successful blob delete (two-phase commit).
	DequeueCleanup(ctx context.Context, before time.Time, limit int) ([]string, error)

	// RemoveCleanup drops an entry from the cleanup index.
	RemoveCleanup(ctx context.Context, blobKey string) error

	// Ping is the probe for /__heartbeat__.
	Ping(ctx context.Context) error

	// Close releases resources (Redis connection, janitor goroutine, ...).
	Close() error
}
