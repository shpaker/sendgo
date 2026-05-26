package portfakes

import (
	"context"
	"errors"
	"time"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// FakeSetFieldCall records a SetField invocation.
type FakeSetFieldCall struct {
	ID    domain.ShareID
	Field string
	Value string
}

// FakeIncrFieldCall records an IncrField invocation.
type FakeIncrFieldCall struct {
	ID    domain.ShareID
	Field string
}

// FakeCleanupCall records an EnqueueCleanup invocation.
type FakeCleanupCall struct {
	BlobKey   string
	ExpiresAt time.Time
}

// FakeDequeueCleanupCall records a DequeueCleanup invocation.
type FakeDequeueCleanupCall struct {
	Before time.Time
	Limit  int
}

// FakeMetaStore is an in-memory port.MetaStore. Shares holds the stored
// records keyed by ShareID; TTLs holds per-share remaining lifetime
// returned by TTL(); Cleanup is a slice (unsorted; the production sorted
// invariant is irrelevant to use cases under test).
type FakeMetaStore struct {
	Shares  map[domain.ShareID]*domain.FileShare
	TTLs    map[domain.ShareID]time.Duration
	Cleanup []FakeCleanupCall

	ErrOnSet            error
	ErrOnGet            error
	ErrOnSetField       error
	ErrOnIncrField      error
	ErrOnDelete         error
	ErrOnTTL            error
	ErrOnEnqueueCleanup error
	ErrOnDequeueCleanup error
	ErrOnRemoveCleanup  error
	ErrOnPing           error
	ErrOnClose          error

	SetCalls            []domain.FileShare
	GetCalls            []domain.ShareID
	SetFieldCalls       []FakeSetFieldCall
	IncrFieldCalls      []FakeIncrFieldCall
	DeleteCalls         []domain.ShareID
	TTLCalls            []domain.ShareID
	EnqueueCleanupCalls []FakeCleanupCall
	DequeueCleanupCalls []FakeDequeueCleanupCall
	RemoveCleanupCalls  []string
	PingCalls           int
	CloseCalls          int
}

// NewFakeMetaStore returns a meta store with initialized empty maps.
func NewFakeMetaStore() *FakeMetaStore {
	return &FakeMetaStore{
		Shares: make(map[domain.ShareID]*domain.FileShare),
		TTLs:   make(map[domain.ShareID]time.Duration),
	}
}

// Set stores a copy of the share and records the call. If a TTL has not
// been pre-seeded for this id, it defaults to expireSeconds.
func (f *FakeMetaStore) Set(ctx context.Context, share *domain.FileShare, expireSeconds int) error {
	f.SetCalls = append(f.SetCalls, *share)
	if f.ErrOnSet != nil {
		return f.ErrOnSet
	}
	clone := *share
	f.Shares[share.ID] = &clone
	if _, ok := f.TTLs[share.ID]; !ok {
		f.TTLs[share.ID] = time.Duration(expireSeconds) * time.Second
	}
	return nil
}

// Get returns a copy of the stored share, or domain.ErrNotFound.
func (f *FakeMetaStore) Get(ctx context.Context, id domain.ShareID) (*domain.FileShare, error) {
	f.GetCalls = append(f.GetCalls, id)
	if f.ErrOnGet != nil {
		return nil, f.ErrOnGet
	}
	s, ok := f.Shares[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	clone := *s
	return &clone, nil
}

// SetField records the call. It does not interpret the field — assertions
// live in the test.
func (f *FakeMetaStore) SetField(ctx context.Context, id domain.ShareID, field, value string) error {
	f.SetFieldCalls = append(f.SetFieldCalls, FakeSetFieldCall{ID: id, Field: field, Value: value})
	return f.ErrOnSetField
}

// IncrField bumps the "dl" counter on the stored share and returns the new
// value. Other fields return an error (matches MemoryStore behavior, since
// production code only increments "dl").
func (f *FakeMetaStore) IncrField(ctx context.Context, id domain.ShareID, field string) (int64, error) {
	f.IncrFieldCalls = append(f.IncrFieldCalls, FakeIncrFieldCall{ID: id, Field: field})
	if f.ErrOnIncrField != nil {
		return 0, f.ErrOnIncrField
	}
	s, ok := f.Shares[id]
	if !ok {
		return 0, domain.ErrNotFound
	}
	switch field {
	case "dl":
		s.DL++
		return int64(s.DL), nil
	default:
		return 0, errors.New("fake meta: unsupported incr field " + field)
	}
}

// Delete removes the share and its TTL entry. Idempotent.
func (f *FakeMetaStore) Delete(ctx context.Context, id domain.ShareID) error {
	f.DeleteCalls = append(f.DeleteCalls, id)
	if f.ErrOnDelete != nil {
		return f.ErrOnDelete
	}
	delete(f.Shares, id)
	delete(f.TTLs, id)
	return nil
}

// TTL returns the value stored in TTLs[id], or domain.ErrNotFound when the
// share is unknown.
func (f *FakeMetaStore) TTL(ctx context.Context, id domain.ShareID) (time.Duration, error) {
	f.TTLCalls = append(f.TTLCalls, id)
	if f.ErrOnTTL != nil {
		return 0, f.ErrOnTTL
	}
	d, ok := f.TTLs[id]
	if !ok {
		return 0, domain.ErrNotFound
	}
	return d, nil
}

// EnqueueCleanup appends an entry; the slice is intentionally unsorted.
func (f *FakeMetaStore) EnqueueCleanup(ctx context.Context, blobKey string, expiresAt time.Time) error {
	entry := FakeCleanupCall{BlobKey: blobKey, ExpiresAt: expiresAt}
	f.EnqueueCleanupCalls = append(f.EnqueueCleanupCalls, entry)
	if f.ErrOnEnqueueCleanup != nil {
		return f.ErrOnEnqueueCleanup
	}
	f.Cleanup = append(f.Cleanup, entry)
	return nil
}

// DequeueCleanup returns up to limit keys whose ExpiresAt ≤ before.
func (f *FakeMetaStore) DequeueCleanup(ctx context.Context, before time.Time, limit int) ([]string, error) {
	f.DequeueCleanupCalls = append(f.DequeueCleanupCalls, FakeDequeueCleanupCall{Before: before, Limit: limit})
	if f.ErrOnDequeueCleanup != nil {
		return nil, f.ErrOnDequeueCleanup
	}
	out := []string{}
	for _, e := range f.Cleanup {
		if e.ExpiresAt.After(before) {
			continue
		}
		out = append(out, e.BlobKey)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// RemoveCleanup drops the first matching entry. Idempotent.
func (f *FakeMetaStore) RemoveCleanup(ctx context.Context, blobKey string) error {
	f.RemoveCleanupCalls = append(f.RemoveCleanupCalls, blobKey)
	if f.ErrOnRemoveCleanup != nil {
		return f.ErrOnRemoveCleanup
	}
	for i, e := range f.Cleanup {
		if e.BlobKey == blobKey {
			f.Cleanup = append(f.Cleanup[:i], f.Cleanup[i+1:]...)
			return nil
		}
	}
	return nil
}

// Ping returns ErrOnPing or nil.
func (f *FakeMetaStore) Ping(ctx context.Context) error {
	f.PingCalls++
	return f.ErrOnPing
}

// Close returns ErrOnClose or nil.
func (f *FakeMetaStore) Close() error {
	f.CloseCalls++
	return f.ErrOnClose
}

var _ port.MetaStore = (*FakeMetaStore)(nil)
