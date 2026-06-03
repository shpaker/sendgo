package meta

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sendgo/sendgo/server/internal/domain"
)

func newTestShare() *domain.FileShare {
	return &domain.FileShare{
		ID:                "abcdef1234567890",
		Owner:             "11112222333344445555",
		EncryptedMetadata: []byte("blob"),
		AuthKey:           []byte("auth-key"),
		Nonce:             []byte("nonce-16-bytes!!"),
		DLimit:            3,
		DL:                0,
		ExpireSeconds:     3600,
		Prefix:            1,
	}
}

func TestMemoryStore_SetGetDelete(t *testing.T) {
	m := NewMemoryStore()
	defer m.Close()
	ctx := context.Background()
	s := newTestShare()

	if err := m.Set(ctx, s, 60); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := m.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != s.ID || got.DLimit != s.DLimit {
		t.Errorf("Get returned wrong share: %+v", got)
	}
	if err := m.Delete(ctx, s.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := m.Get(ctx, s.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get after Delete: want ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_TTLExpires(t *testing.T) {
	m := NewMemoryStore()
	defer m.Close()
	ctx := context.Background()
	s := newTestShare()
	if err := m.Set(ctx, s, 0); err != nil { // expireSeconds=0 → expired immediately
		t.Fatalf("Set: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if _, err := m.Get(ctx, s.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for expired, got %v", err)
	}
}

func TestMemoryStore_IncrField(t *testing.T) {
	m := NewMemoryStore()
	defer m.Close()
	ctx := context.Background()
	s := newTestShare()
	_ = m.Set(ctx, s, 60)

	v, err := m.IncrField(ctx, s.ID, "dl")
	if err != nil {
		t.Fatalf("IncrField: %v", err)
	}
	if v != 1 {
		t.Errorf("IncrField dl: want 1, got %d", v)
	}
	v, _ = m.IncrField(ctx, s.ID, "dl")
	if v != 2 {
		t.Errorf("IncrField dl: want 2, got %d", v)
	}

	got, _ := m.Get(ctx, s.ID)
	if got.DL != 2 {
		t.Errorf("share.DL after Incr: want 2, got %d", got.DL)
	}
}

func TestMemoryStore_CleanupIndex(t *testing.T) {
	m := NewMemoryStore()
	defer m.Close()
	ctx := context.Background()

	now := time.Now()
	_ = m.EnqueueCleanup(ctx, "1-old", now.Add(-time.Hour))
	_ = m.EnqueueCleanup(ctx, "1-future", now.Add(time.Hour))
	_ = m.EnqueueCleanup(ctx, "1-now", now)

	due, err := m.DequeueCleanup(ctx, now.Add(time.Second), 10)
	if err != nil {
		t.Fatalf("DequeueCleanup: %v", err)
	}
	if len(due) != 2 {
		t.Errorf("want 2 due entries, got %d (%v)", len(due), due)
	}
	// Expect 1-old first (sorted in ascending order).
	if due[0] != "1-old" {
		t.Errorf("first due entry: want 1-old, got %q", due[0])
	}

	_ = m.RemoveCleanup(ctx, "1-old")
	_ = m.RemoveCleanup(ctx, "1-now")
	due2, _ := m.DequeueCleanup(ctx, now.Add(time.Second), 10)
	if len(due2) != 0 {
		t.Errorf("after Remove: want 0 due, got %d (%v)", len(due2), due2)
	}
}
