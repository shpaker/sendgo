package portfakes_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port/portfakes"
)

func TestFakeClock_AdvanceMovesTime(t *testing.T) {
	c := portfakes.NewFakeClock(time.Unix(1000, 0))
	if got := c.Now().Unix(); got != 1000 {
		t.Fatalf("Now=%d want 1000", got)
	}
	c.Advance(30 * time.Second)
	if got := c.Now().Unix(); got != 1030 {
		t.Fatalf("Now=%d want 1030", got)
	}
}

func TestFakeTokenGenerator_PopsQueuesThenSynthesizes(t *testing.T) {
	tg := portfakes.NewFakeTokenGenerator(
		[]domain.ShareID{"id1"},
		[]domain.OwnerToken{"own1"},
		[][]byte{{0xAA}},
	)

	id, _ := tg.ShareID()
	own, _ := tg.OwnerToken()
	nonce, _ := tg.Nonce()
	if id != "id1" || own != "own1" || nonce[0] != 0xAA {
		t.Fatalf("preloaded values not returned: %s/%s/%x", id, own, nonce)
	}

	// Queues empty — synthetic values with monotonic counter.
	id2, _ := tg.ShareID()
	own2, _ := tg.OwnerToken()
	nonce2, _ := tg.Nonce()
	if id2 != "auto-fake-id-2" || own2 != "auto-fake-owner-2" || nonce2[0] != 2 {
		t.Fatalf("synthetic values wrong: %s/%s/%x", id2, own2, nonce2)
	}
}

func TestFakeTokenGenerator_ErrorInjection(t *testing.T) {
	tg := &portfakes.FakeTokenGenerator{
		ErrOnShareID:    errors.New("id"),
		ErrOnOwnerToken: errors.New("own"),
		ErrOnNonce:      errors.New("nonce"),
	}
	if _, err := tg.ShareID(); err == nil {
		t.Errorf("ShareID: expected error")
	}
	if _, err := tg.OwnerToken(); err == nil {
		t.Errorf("OwnerToken: expected error")
	}
	if _, err := tg.Nonce(); err == nil {
		t.Errorf("Nonce: expected error")
	}
}

func TestFakeBlobStorage_RoundTrip(t *testing.T) {
	ctx := context.Background()
	b := portfakes.NewFakeBlobStorage()
	body := []byte("hello world")

	if err := b.Put(ctx, "k1", bytes.NewReader(body), 60); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if len(b.PutCalls) != 1 || !bytes.Equal(b.PutCalls[0].Body, body) || b.PutCalls[0].ExpireSeconds != 60 {
		t.Errorf("PutCalls not recorded: %+v", b.PutCalls)
	}

	size, exists, err := b.Head(ctx, "k1")
	if err != nil || !exists || size != int64(len(body)) {
		t.Errorf("Head: size=%d exists=%v err=%v", size, exists, err)
	}

	n, err := b.Length(ctx, "k1")
	if err != nil || n != int64(len(body)) {
		t.Errorf("Length: n=%d err=%v", n, err)
	}

	rc, err := b.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, body) {
		t.Errorf("Get body = %q want %q", got, body)
	}

	if err := b.Delete(ctx, "k1"); err != nil {
		t.Errorf("Delete: %v", err)
	}
	if _, err := b.Get(ctx, "k1"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get after delete = %v want ErrNotFound", err)
	}
}

func TestFakeBlobStorage_MissingAndErrors(t *testing.T) {
	ctx := context.Background()
	b := portfakes.NewFakeBlobStorage()

	if _, _, err := b.Head(ctx, "missing"); err != nil {
		t.Errorf("Head missing: err=%v want nil", err)
	}
	if _, err := b.Length(ctx, "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Length missing = %v want ErrNotFound", err)
	}
	// Delete missing is idempotent.
	if err := b.Delete(ctx, "missing"); err != nil {
		t.Errorf("Delete missing: %v", err)
	}

	b.ErrOnPut = errors.New("put down")
	b.ErrOnGet = errors.New("get down")
	b.ErrOnHead = errors.New("head down")
	b.ErrOnDelete = errors.New("del down")
	b.ErrOnLength = errors.New("len down")
	b.ErrOnPing = errors.New("ping down")

	if err := b.Put(ctx, "k", bytes.NewReader([]byte("x")), 0); err == nil {
		t.Error("Put: expected error")
	}
	if len(b.PutCalls) != 1 {
		t.Error("Put call should still be recorded under error")
	}
	if _, err := b.Get(ctx, "k"); err == nil {
		t.Error("Get: expected error")
	}
	if _, _, err := b.Head(ctx, "k"); err == nil {
		t.Error("Head: expected error")
	}
	if err := b.Delete(ctx, "k"); err == nil {
		t.Error("Delete: expected error")
	}
	if _, err := b.Length(ctx, "k"); err == nil {
		t.Error("Length: expected error")
	}
	if err := b.Ping(ctx); err == nil {
		t.Error("Ping: expected error")
	}
	if b.PingCalls != 1 {
		t.Errorf("PingCalls=%d want 1", b.PingCalls)
	}
}

func TestFakeMetaStore_SetGetDelete(t *testing.T) {
	ctx := context.Background()
	m := portfakes.NewFakeMetaStore()
	share := &domain.FileShare{ID: "abc1234567", DLimit: 3}

	if err := m.Set(ctx, share, 60); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := m.Get(ctx, share.ID)
	if err != nil || got.ID != share.ID || got.DLimit != 3 {
		t.Errorf("Get = %+v err=%v", got, err)
	}
	// Mutating the returned copy must not affect storage.
	got.DLimit = 99
	got2, _ := m.Get(ctx, share.ID)
	if got2.DLimit != 3 {
		t.Errorf("Get returned shared reference, DLimit drifted to %d", got2.DLimit)
	}

	ttl, err := m.TTL(ctx, share.ID)
	if err != nil || ttl != 60*time.Second {
		t.Errorf("TTL = %v err=%v", ttl, err)
	}

	if err := m.Delete(ctx, share.ID); err != nil {
		t.Errorf("Delete: %v", err)
	}
	if _, err := m.Get(ctx, share.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Get after delete = %v want ErrNotFound", err)
	}
	if _, err := m.TTL(ctx, share.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("TTL after delete = %v want ErrNotFound", err)
	}
}

func TestFakeMetaStore_TTLPreseed(t *testing.T) {
	ctx := context.Background()
	m := portfakes.NewFakeMetaStore()
	share := &domain.FileShare{ID: "abc1234567"}
	m.TTLs[share.ID] = 5 * time.Second

	if err := m.Set(ctx, share, 60); err != nil {
		t.Fatalf("Set: %v", err)
	}
	ttl, _ := m.TTL(ctx, share.ID)
	if ttl != 5*time.Second {
		t.Errorf("TTL respects pre-seed: got %v want 5s", ttl)
	}
}

func TestFakeMetaStore_IncrField(t *testing.T) {
	ctx := context.Background()
	m := portfakes.NewFakeMetaStore()
	share := &domain.FileShare{ID: "abc1234567", DL: 2}
	_ = m.Set(ctx, share, 60)

	n, err := m.IncrField(ctx, share.ID, "dl")
	if err != nil || n != 3 {
		t.Errorf("IncrField dl = %d err=%v want 3", n, err)
	}

	if _, err := m.IncrField(ctx, share.ID, "unknown"); err == nil {
		t.Error("IncrField unknown: expected error")
	}

	if _, err := m.IncrField(ctx, "missing", "dl"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("IncrField missing = %v want ErrNotFound", err)
	}
}

func TestFakeMetaStore_SetField_RecordsOnly(t *testing.T) {
	ctx := context.Background()
	m := portfakes.NewFakeMetaStore()

	if err := m.SetField(ctx, "id", "pwd", "true"); err != nil {
		t.Errorf("SetField: %v", err)
	}
	if len(m.SetFieldCalls) != 1 || m.SetFieldCalls[0].Field != "pwd" {
		t.Errorf("SetFieldCalls not recorded: %+v", m.SetFieldCalls)
	}

	m.ErrOnSetField = errors.New("boom")
	if err := m.SetField(ctx, "id", "auth", "x"); err == nil {
		t.Error("SetField under ErrOnSetField: expected error")
	}
	if len(m.SetFieldCalls) != 2 {
		t.Error("SetField call should be recorded under error")
	}
}

func TestFakeMetaStore_CleanupIndex(t *testing.T) {
	ctx := context.Background()
	m := portfakes.NewFakeMetaStore()
	now := time.Unix(1_000_000, 0)

	_ = m.EnqueueCleanup(ctx, "k1", now)
	_ = m.EnqueueCleanup(ctx, "k2", now.Add(10*time.Second))
	_ = m.EnqueueCleanup(ctx, "k3", now.Add(20*time.Second))

	keys, err := m.DequeueCleanup(ctx, now.Add(15*time.Second), 10)
	if err != nil {
		t.Fatalf("DequeueCleanup: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("DequeueCleanup len=%d want 2 (k1,k2): %v", len(keys), keys)
	}

	_ = m.RemoveCleanup(ctx, "k2")
	keys2, _ := m.DequeueCleanup(ctx, now.Add(15*time.Second), 10)
	if len(keys2) != 1 || keys2[0] != "k1" {
		t.Errorf("after RemoveCleanup: %v want [k1]", keys2)
	}

	// Limit honored.
	_ = m.EnqueueCleanup(ctx, "k4", now)
	keys3, _ := m.DequeueCleanup(ctx, now.Add(15*time.Second), 1)
	if len(keys3) != 1 {
		t.Errorf("limit=1 returned %d keys", len(keys3))
	}

	// RemoveCleanup on missing key is a no-op.
	if err := m.RemoveCleanup(ctx, "missing"); err != nil {
		t.Errorf("RemoveCleanup missing: %v", err)
	}
}

func TestFakeMetaStore_ErrorInjection(t *testing.T) {
	ctx := context.Background()
	m := portfakes.NewFakeMetaStore()

	m.ErrOnSet = errors.New("set")
	m.ErrOnGet = errors.New("get")
	m.ErrOnDelete = errors.New("del")
	m.ErrOnTTL = errors.New("ttl")
	m.ErrOnIncrField = errors.New("incr")
	m.ErrOnEnqueueCleanup = errors.New("enq")
	m.ErrOnDequeueCleanup = errors.New("deq")
	m.ErrOnRemoveCleanup = errors.New("rm")
	m.ErrOnPing = errors.New("ping")
	m.ErrOnClose = errors.New("close")

	if err := m.Set(ctx, &domain.FileShare{ID: "x"}, 1); err == nil {
		t.Error("Set: expected error")
	}
	if _, err := m.Get(ctx, "x"); err == nil {
		t.Error("Get: expected error")
	}
	if err := m.Delete(ctx, "x"); err == nil {
		t.Error("Delete: expected error")
	}
	if _, err := m.TTL(ctx, "x"); err == nil {
		t.Error("TTL: expected error")
	}
	if _, err := m.IncrField(ctx, "x", "dl"); err == nil {
		t.Error("IncrField: expected error")
	}
	if err := m.EnqueueCleanup(ctx, "k", time.Now()); err == nil {
		t.Error("EnqueueCleanup: expected error")
	}
	if _, err := m.DequeueCleanup(ctx, time.Now(), 1); err == nil {
		t.Error("DequeueCleanup: expected error")
	}
	if err := m.RemoveCleanup(ctx, "k"); err == nil {
		t.Error("RemoveCleanup: expected error")
	}
	if err := m.Ping(ctx); err == nil {
		t.Error("Ping: expected error")
	}
	if err := m.Close(); err == nil {
		t.Error("Close: expected error")
	}
	if m.CloseCalls != 1 || m.PingCalls != 1 {
		t.Errorf("counters wrong: ping=%d close=%d", m.PingCalls, m.CloseCalls)
	}
}
