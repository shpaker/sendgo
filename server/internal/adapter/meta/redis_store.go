package meta

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/sendgo/sendgo/server/internal/crypto"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// RedisStore is the Redis-backed MetaStore implementation. Key schema is 1:1
// with the Node code:
//   - hash at key <share_id> with fields prefix/owner/metadata/auth/nonce/dlimit/dl/pwd
//   - EXPIRE <share_id> <ttl_seconds>
//
// The cleanup index is a separate sorted set sendgo:cleanup:queue
// (score=unix_ts, member=blob_key).
type RedisStore struct {
	c            *redis.Client
	cleanupQueue string
}

const defaultCleanupQueue = "sendgo:cleanup:queue"

// NewRedisStore parses the DSN and pings the server. If the ping fails, an
// error is returned so main can fail with a clear message rather than dying
// at runtime on the first request.
func NewRedisStore(ctx context.Context, dsn string) (*RedisStore, error) {
	opt, err := redis.ParseURL(dsn)
	if err != nil {
		return nil, fmt.Errorf("redis dsn: %w", err)
	}
	c := redis.NewClient(opt)
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return &RedisStore{c: c, cleanupQueue: defaultCleanupQueue}, nil
}

func (r *RedisStore) Close() error { return r.c.Close() }

func (r *RedisStore) Ping(ctx context.Context) error { return r.c.Ping(ctx).Err() }

// Set runs HMSET + EXPIRE in a pipeline (one network roundtrip). Fields are
// serialized the same way the Node server does it.
func (r *RedisStore) Set(ctx context.Context, s *domain.FileShare, expireSeconds int) error {
	// Use URL-safe base64 without padding on write — the same format the
	// frontend sends (see crypto.EncodeB64). On read, applyField decodes
	// leniently (any flavor).
	fields := map[string]any{
		"prefix":   strconv.Itoa(s.Prefix),
		"owner":    string(s.Owner),
		"metadata": crypto.EncodeB64(s.EncryptedMetadata),
		"auth":     crypto.EncodeB64(s.AuthKey),
		"nonce":    crypto.EncodeB64(s.Nonce),
		"dlimit":   strconv.Itoa(s.DLimit),
		"dl":       strconv.Itoa(s.DL),
		"pwd":      strconv.FormatBool(s.Password),
	}
	pipe := r.c.TxPipeline()
	pipe.HSet(ctx, string(s.ID), fields)
	pipe.Expire(ctx, string(s.ID), time.Duration(expireSeconds)*time.Second)
	_, err := pipe.Exec(ctx)
	return err
}

func (r *RedisStore) Get(ctx context.Context, id domain.ShareID) (*domain.FileShare, error) {
	res, err := r.c.HGetAll(ctx, string(id)).Result()
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, domain.ErrNotFound
	}
	share := &domain.FileShare{ID: id}
	for k, v := range res {
		if err := applyField(share, k, v); err != nil {
			return nil, fmt.Errorf("redis hash field %q: %w", k, err)
		}
	}
	return share, nil
}

func (r *RedisStore) SetField(ctx context.Context, id domain.ShareID, field, value string) error {
	return r.c.HSet(ctx, string(id), field, value).Err()
}

func (r *RedisStore) IncrField(ctx context.Context, id domain.ShareID, field string) (int64, error) {
	v, err := r.c.HIncrBy(ctx, string(id), field, 1).Result()
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *RedisStore) Delete(ctx context.Context, id domain.ShareID) error {
	return r.c.Del(ctx, string(id)).Err()
}

func (r *RedisStore) TTL(ctx context.Context, id domain.ShareID) (time.Duration, error) {
	d, err := r.c.TTL(ctx, string(id)).Result()
	if err != nil {
		return 0, err
	}
	if d == -2*time.Second { // -2 = key does not exist (go-redis maps it to -2s)
		return 0, domain.ErrNotFound
	}
	return d, nil
}

// --- Cleanup index ---

func (r *RedisStore) EnqueueCleanup(ctx context.Context, blobKey string, expiresAt time.Time) error {
	return r.c.ZAdd(ctx, r.cleanupQueue, redis.Z{
		Score:  float64(expiresAt.Unix()),
		Member: blobKey,
	}).Err()
}

func (r *RedisStore) DequeueCleanup(ctx context.Context, before time.Time, limit int) ([]string, error) {
	return r.c.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     r.cleanupQueue,
		Start:   "0",
		Stop:    strconv.FormatInt(before.Unix(), 10),
		ByScore: true,
		Offset:  0,
		Count:   int64(limit),
	}).Result()
}

func (r *RedisStore) RemoveCleanup(ctx context.Context, blobKey string) error {
	return r.c.ZRem(ctx, r.cleanupQueue, blobKey).Err()
}

// Sanity check: interface fully implemented.
var _ port.MetaStore = (*RedisStore)(nil)

// Sometimes redis.Nil shows up in place of ErrNotFound — normalize in one
// place.
//
//nolint:unused
func wrapNotFound(err error) error {
	if errors.Is(err, redis.Nil) {
		return domain.ErrNotFound
	}
	return err
}
