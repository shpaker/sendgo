// Package meta provides port.MetaStore implementations: in-memory (default)
// and Redis (when a DSN is configured).
package meta

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/sendgo/sendgo/server/internal/crypto"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// MemoryStore is a simple in-process implementation. State is lost on restart.
// Used when CLI.RedisDSN is empty.
type MemoryStore struct {
	mu      sync.RWMutex
	shares  map[domain.ShareID]*storedShare
	cleanup []cleanupEntry // sorted by expiresAt asc; kept small (sorted slice)
	janitor *time.Ticker
	done    chan struct{}
}

type storedShare struct {
	share     *domain.FileShare
	expiresAt time.Time
}

type cleanupEntry struct {
	expiresAt time.Time
	blobKey   string
}

// NewMemoryStore starts a janitor goroutine that frees memory from expired
// shares. The janitor runs independently from the cleanup runner, so the map
// does not grow unbounded even without --cleanup-enabled.
func NewMemoryStore() *MemoryStore {
	m := &MemoryStore{
		shares:  make(map[domain.ShareID]*storedShare),
		cleanup: nil,
		janitor: time.NewTicker(time.Minute),
		done:    make(chan struct{}),
	}
	go m.runJanitor()
	return m
}

func (m *MemoryStore) runJanitor() {
	for {
		select {
		case <-m.janitor.C:
			now := time.Now()
			m.mu.Lock()
			for id, ss := range m.shares {
				if !ss.expiresAt.After(now) {
					delete(m.shares, id)
				}
			}
			m.mu.Unlock()
		case <-m.done:
			return
		}
	}
}

func (m *MemoryStore) Close() error {
	m.janitor.Stop()
	close(m.done)
	return nil
}

func (m *MemoryStore) Ping(ctx context.Context) error { return nil }

// --- Set / Get / mutators ---

func (m *MemoryStore) Set(ctx context.Context, share *domain.FileShare, expireSeconds int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	clone := *share
	m.shares[share.ID] = &storedShare{
		share:     &clone,
		expiresAt: time.Now().Add(time.Duration(expireSeconds) * time.Second),
	}
	return nil
}

func (m *MemoryStore) Get(ctx context.Context, id domain.ShareID) (*domain.FileShare, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ss, ok := m.shares[id]
	if !ok || !ss.expiresAt.After(time.Now()) {
		return nil, domain.ErrNotFound
	}
	clone := *ss.share
	return &clone, nil
}

// SetField updates a single field. Field names match the Redis variant so
// use cases stay agnostic of the implementation.
func (m *MemoryStore) SetField(ctx context.Context, id domain.ShareID, field, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ss, ok := m.shares[id]
	if !ok {
		return domain.ErrNotFound
	}
	return applyField(ss.share, field, value)
}

func (m *MemoryStore) IncrField(ctx context.Context, id domain.ShareID, field string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ss, ok := m.shares[id]
	if !ok {
		return 0, domain.ErrNotFound
	}
	switch field {
	case "dl":
		ss.share.DL++
		return int64(ss.share.DL), nil
	default:
		return 0, errors.New("memory meta: unsupported incr field " + field)
	}
}

func (m *MemoryStore) Delete(ctx context.Context, id domain.ShareID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.shares, id)
	return nil
}

func (m *MemoryStore) TTL(ctx context.Context, id domain.ShareID) (time.Duration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ss, ok := m.shares[id]
	if !ok {
		return 0, domain.ErrNotFound
	}
	d := time.Until(ss.expiresAt)
	if d < 0 {
		return 0, domain.ErrNotFound
	}
	return d, nil
}

// --- Cleanup index: sorted slice of {expiresAt, blobKey} ---

func (m *MemoryStore) EnqueueCleanup(ctx context.Context, blobKey string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Binary search + insert — O(n) per insert, but n is small for our use case.
	idx := sort.Search(len(m.cleanup), func(i int) bool {
		return m.cleanup[i].expiresAt.After(expiresAt)
	})
	m.cleanup = append(m.cleanup, cleanupEntry{})
	copy(m.cleanup[idx+1:], m.cleanup[idx:])
	m.cleanup[idx] = cleanupEntry{expiresAt: expiresAt, blobKey: blobKey}
	return nil
}

func (m *MemoryStore) DequeueCleanup(ctx context.Context, before time.Time, limit int) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []string{}
	for _, e := range m.cleanup {
		if e.expiresAt.After(before) {
			break // sorted — everything after is in the future
		}
		out = append(out, e.blobKey)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *MemoryStore) RemoveCleanup(ctx context.Context, blobKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.cleanup {
		if e.blobKey == blobKey {
			m.cleanup = append(m.cleanup[:i], m.cleanup[i+1:]...)
			return nil
		}
	}
	return nil // idempotent
}

// applyField is the shared mapper from string value → concrete FileShare
// field. Used both by MemoryStore (for in-memory updates via SetField) and to
// unpack the Redis hash on read. Field names match the Node schema in
// server/storage/index.js. Base64 values are decoded leniently: the frontend
// sends URL-safe without padding, legacy Node accepted standard — both forms
// are valid.
func applyField(s *domain.FileShare, field, value string) error {
	switch field {
	case "owner":
		s.Owner = domain.OwnerToken(value)
	case "metadata":
		b, err := crypto.DecodeB64(value)
		if err != nil {
			return err
		}
		s.EncryptedMetadata = b
	case "auth":
		b, err := crypto.DecodeB64(value)
		if err != nil {
			return err
		}
		s.AuthKey = b
	case "nonce":
		b, err := crypto.DecodeB64(value)
		if err != nil {
			return err
		}
		s.Nonce = b
	case "pwd":
		s.Password = value == "true" || value == "1"
	case "dlimit":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		s.DLimit = n
	case "dl":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		s.DL = n
	case "prefix":
		n, err := strconv.Atoi(value)
		if err != nil {
			return err
		}
		s.Prefix = n
	default:
		return errors.New("unknown field: " + field)
	}
	return nil
}

// Compile-time interface check.
var _ port.MetaStore = (*MemoryStore)(nil)
