package portfakes

import (
	"bytes"
	"context"
	"io"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// FakePutCall records the arguments of one Put invocation, including the
// fully drained body so the test can assert on the bytes that flowed through.
type FakePutCall struct {
	Key           string
	ExpireSeconds int
	Body          []byte
}

// FakeBlobStorage is an in-memory port.BlobStorage. Objects map holds the
// stored bytes; tests can pre-load it or read it back after Execute.
type FakeBlobStorage struct {
	Objects map[string][]byte

	ErrOnPut    error
	ErrOnGet    error
	ErrOnHead   error
	ErrOnDelete error
	ErrOnLength error
	ErrOnPing   error

	PutCalls    []FakePutCall
	GetCalls    []string
	HeadCalls   []string
	DeleteCalls []string
	LengthCalls []string
	PingCalls   int
}

// NewFakeBlobStorage returns a fake with an initialized empty map.
func NewFakeBlobStorage() *FakeBlobStorage {
	return &FakeBlobStorage{Objects: make(map[string][]byte)}
}

// Put drains r into Objects[key] and records the call. If ErrOnPut is set,
// it returns the error AFTER recording (matches real-world ordering: the
// caller observes the failed write but the test still sees the attempt).
// The body is read fully even when ErrOnPut is set, so tests can inspect
// what the use case streamed up to the failure point.
func (f *FakeBlobStorage) Put(ctx context.Context, key string, r io.Reader, expireSeconds int) error {
	body, _ := io.ReadAll(r)
	f.PutCalls = append(f.PutCalls, FakePutCall{Key: key, ExpireSeconds: expireSeconds, Body: append([]byte(nil), body...)})
	if f.ErrOnPut != nil {
		return f.ErrOnPut
	}
	f.Objects[key] = body
	return nil
}

// Get returns a ReadCloser over Objects[key]. Missing → domain.ErrNotFound.
func (f *FakeBlobStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	f.GetCalls = append(f.GetCalls, key)
	if f.ErrOnGet != nil {
		return nil, f.ErrOnGet
	}
	body, ok := f.Objects[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

// Head returns size + existence flag for the key.
func (f *FakeBlobStorage) Head(ctx context.Context, key string) (int64, bool, error) {
	f.HeadCalls = append(f.HeadCalls, key)
	if f.ErrOnHead != nil {
		return 0, false, f.ErrOnHead
	}
	body, ok := f.Objects[key]
	if !ok {
		return 0, false, nil
	}
	return int64(len(body)), true, nil
}

// Delete is idempotent: removing a missing key returns nil.
func (f *FakeBlobStorage) Delete(ctx context.Context, key string) error {
	f.DeleteCalls = append(f.DeleteCalls, key)
	if f.ErrOnDelete != nil {
		return f.ErrOnDelete
	}
	delete(f.Objects, key)
	return nil
}

// Length returns the size of an existing object; missing → ErrNotFound.
func (f *FakeBlobStorage) Length(ctx context.Context, key string) (int64, error) {
	f.LengthCalls = append(f.LengthCalls, key)
	if f.ErrOnLength != nil {
		return 0, f.ErrOnLength
	}
	body, ok := f.Objects[key]
	if !ok {
		return 0, domain.ErrNotFound
	}
	return int64(len(body)), nil
}

// Ping returns ErrOnPing or nil.
func (f *FakeBlobStorage) Ping(ctx context.Context) error {
	f.PingCalls++
	return f.ErrOnPing
}

var _ port.BlobStorage = (*FakeBlobStorage)(nil)
