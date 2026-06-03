package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port/portfakes"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

func TestStreamUpload_HappyPath(t *testing.T) {
	ctx := context.Background()
	clk := portfakes.NewFakeClock(time.Unix(1_700_000_000, 0))
	blob := portfakes.NewFakeBlobStorage()
	meta := portfakes.NewFakeMetaStore()
	uc := &usecase.StreamUpload{Blob: blob, Meta: meta, MaxSize: 1024, Clock: clk}
	share := &domain.FileShare{ID: "id1234567890", Prefix: 1, ExpireSeconds: 60}
	body := bytes.Repeat([]byte("x"), 100)

	n, err := uc.Execute(ctx, share, "1-id1234567890", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if n != 100 || share.SizeBytes != 100 {
		t.Errorf("n=%d share.SizeBytes=%d want 100", n, share.SizeBytes)
	}
	if len(blob.PutCalls) != 1 || blob.PutCalls[0].Key != "1-id1234567890" || blob.PutCalls[0].ExpireSeconds != 60 {
		t.Errorf("PutCalls=%+v", blob.PutCalls)
	}
	if len(meta.SetCalls) != 1 {
		t.Errorf("expected exactly 1 meta.Set call, got %d", len(meta.SetCalls))
	}
	if len(meta.EnqueueCleanupCalls) != 1 {
		t.Errorf("expected exactly 1 EnqueueCleanup call, got %d", len(meta.EnqueueCleanupCalls))
	}
	want := time.Unix(1_700_000_060, 0)
	if !meta.EnqueueCleanupCalls[0].ExpiresAt.Equal(want) {
		t.Errorf("EnqueueCleanup expiresAt=%v want %v", meta.EnqueueCleanupCalls[0].ExpiresAt, want)
	}
	if len(blob.DeleteCalls) != 0 {
		t.Errorf("blob should not be deleted on happy path")
	}
}

func TestStreamUpload_BodyTooLarge(t *testing.T) {
	ctx := context.Background()
	blob := portfakes.NewFakeBlobStorage()
	meta := portfakes.NewFakeMetaStore()
	uc := &usecase.StreamUpload{Blob: blob, Meta: meta, MaxSize: 10, Clock: portfakes.NewFakeClock(time.Now())}
	share := &domain.FileShare{ID: "id1234567890", Prefix: 1, ExpireSeconds: 60}

	// 11 bytes > MaxSize=10 → LimitReader allows MaxSize+1, then the use case bails.
	body := bytes.Repeat([]byte("x"), 11)
	n, err := uc.Execute(ctx, share, "1-id1234567890", bytes.NewReader(body))
	if !errors.Is(err, domain.ErrPayloadTooLarge) {
		t.Fatalf("err=%v want ErrPayloadTooLarge", err)
	}
	if n != 11 {
		t.Errorf("n=%d want 11 (MaxSize+1)", n)
	}
	if len(blob.DeleteCalls) != 1 {
		t.Errorf("expected blob to be deleted after overrun, got %v", blob.DeleteCalls)
	}
	if len(meta.SetCalls) != 0 {
		t.Errorf("meta should not be touched on overrun")
	}
}

func TestStreamUpload_BlobPutError(t *testing.T) {
	blob := portfakes.NewFakeBlobStorage()
	blob.ErrOnPut = errors.New("s3 down")
	meta := portfakes.NewFakeMetaStore()
	uc := &usecase.StreamUpload{Blob: blob, Meta: meta, MaxSize: 1024, Clock: portfakes.NewFakeClock(time.Now())}
	share := &domain.FileShare{ID: "id1234567890", Prefix: 1, ExpireSeconds: 60}

	_, err := uc.Execute(context.Background(), share, "1-id1234567890", bytes.NewReader([]byte("hi")))
	if err == nil {
		t.Fatal("expected error")
	}
	if len(meta.SetCalls) != 0 {
		t.Errorf("meta must not be written when Put fails")
	}
	if len(blob.DeleteCalls) != 0 {
		t.Errorf("Put failure path must not call Delete: %v", blob.DeleteCalls)
	}
}

func TestStreamUpload_MetaSetErrorRollsBackBlob(t *testing.T) {
	blob := portfakes.NewFakeBlobStorage()
	meta := portfakes.NewFakeMetaStore()
	meta.ErrOnSet = errors.New("redis down")
	uc := &usecase.StreamUpload{Blob: blob, Meta: meta, MaxSize: 1024, Clock: portfakes.NewFakeClock(time.Now())}
	share := &domain.FileShare{ID: "id1234567890", Prefix: 1, ExpireSeconds: 60}

	n, err := uc.Execute(context.Background(), share, "1-id1234567890", bytes.NewReader([]byte("hello")))
	if err == nil {
		t.Fatal("expected error")
	}
	if n != 5 {
		t.Errorf("n=%d want 5", n)
	}
	if len(blob.DeleteCalls) != 1 || blob.DeleteCalls[0] != "1-id1234567890" {
		t.Errorf("expected blob rollback delete, got %v", blob.DeleteCalls)
	}
	if len(meta.EnqueueCleanupCalls) != 0 {
		t.Errorf("EnqueueCleanup must not be called when Set failed")
	}
}

func TestStreamUpload_EnqueueCleanupErrorIsNonFatal(t *testing.T) {
	blob := portfakes.NewFakeBlobStorage()
	meta := portfakes.NewFakeMetaStore()
	meta.ErrOnEnqueueCleanup = errors.New("cleanup down")
	uc := &usecase.StreamUpload{Blob: blob, Meta: meta, MaxSize: 1024, Clock: portfakes.NewFakeClock(time.Now())}
	share := &domain.FileShare{ID: "id1234567890", Prefix: 1, ExpireSeconds: 60}

	if _, err := uc.Execute(context.Background(), share, "1-id1234567890", bytes.NewReader([]byte("hi"))); err != nil {
		t.Fatalf("EnqueueCleanup failure must be swallowed, got %v", err)
	}
	// Blob and meta should both be intact.
	if len(blob.DeleteCalls) != 0 {
		t.Errorf("blob should remain after non-fatal cleanup-enqueue failure")
	}
	if len(meta.SetCalls) != 1 {
		t.Errorf("meta.Set should still be recorded")
	}
}
