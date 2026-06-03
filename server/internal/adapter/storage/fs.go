// Package storage provides port.BlobStorage implementations: filesystem (fs.go)
// and S3-compatible (s3.go). Selection is done by factory.go based on config.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/sendgo/sendgo/server/internal/domain"
)

// FSStorage is a bare-bones filesystem store. Default when `--s3-bucket` is
// empty. Matches server/storage/fs.js (Node) 1:1: the key is the file name
// within the directory.
type FSStorage struct {
	dir string
}

// keyRe guards against path traversal. Our keys are formed as
// "{prefix}-{share_id}" — digits/letters/dashes only — and this regex
// enforces that.
var keyRe = regexp.MustCompile(`^[0-9a-zA-Z_-]+$`)

// NewFSStorage ensures the directory exists (mkdir -p) and returns the
// adapter. If dir is empty, falls back to os.TempDir() and creates a send-*
// directory there (same as Node).
func NewFSStorage(dir string) (*FSStorage, error) {
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "send-")
		if err != nil {
			return nil, fmt.Errorf("fs storage: mkdtemp: %w", err)
		}
	} else {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("fs storage: mkdir %q: %w", dir, err)
		}
	}
	return &FSStorage{dir: dir}, nil
}

func (s *FSStorage) path(key string) (string, error) {
	if !keyRe.MatchString(key) {
		return "", fmt.Errorf("%w: invalid blob key %q", domain.ErrBadRequest, key)
	}
	return filepath.Join(s.dir, key), nil
}

// Put performs an atomic write: temp file next to the target + rename. If the
// copy fails, the temp file is removed. expireSeconds is ignored (FS has no
// native TTL — the meta store owns expiry, and the cleanup runner sweeps
// blobs).
func (s *FSStorage) Put(ctx context.Context, key string, r io.Reader, expireSeconds int) error {
	final, err := s.path(key)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "tmp-*")
	if err != nil {
		return fmt.Errorf("fs put: create temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("fs put: copy: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("fs put: fsync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("fs put: close temp: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		cleanup()
		return fmt.Errorf("fs put: rename: %w", err)
	}
	_ = ctx // we're synchronous — context isn't used
	return nil
}

func (s *FSStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	p, err := s.path(key)
	if err != nil {
		return nil, err
	}
	// G304 is not applicable: s.path validates `key` against a strict regex
	// (`^[0-9a-zA-Z_-]+$`) before joining it with s.dir, so a caller-supplied
	// path can't escape the storage directory.
	f, err := os.Open(p) //nolint:gosec // G304: validated above
	if errors.Is(err, fs.ErrNotExist) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("fs get: %w", err)
	}
	_ = ctx
	return f, nil
}

func (s *FSStorage) Head(ctx context.Context, key string) (int64, bool, error) {
	p, err := s.path(key)
	if err != nil {
		return 0, false, err
	}
	st, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("fs head: %w", err)
	}
	_ = ctx
	return st.Size(), true, nil
}

func (s *FSStorage) Delete(ctx context.Context, key string) error {
	p, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("fs delete: %w", err)
	}
	_ = ctx
	return nil
}

func (s *FSStorage) Length(ctx context.Context, key string) (int64, error) {
	size, ok, err := s.Head(ctx, key)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, domain.ErrNotFound
	}
	return size, nil
}

func (s *FSStorage) Ping(ctx context.Context) error {
	_, err := os.Stat(s.dir)
	if err != nil {
		return fmt.Errorf("fs ping: %w", err)
	}
	return nil
}
