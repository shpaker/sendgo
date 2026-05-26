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

func TestDeleteShare_HappyPath(t *testing.T) {
	ctx := context.Background()
	blob := portfakes.NewFakeBlobStorage()
	meta := portfakes.NewFakeMetaStore()
	share := &domain.FileShare{ID: "id1234567890", Prefix: 7}
	_ = blob.Put(ctx, "7-id1234567890", bytes.NewReader([]byte("x")), 0)
	_ = meta.Set(ctx, share, 60)
	_ = meta.EnqueueCleanup(ctx, "7-id1234567890", time.Unix(2_000_000_000, 0))

	uc := &usecase.DeleteShare{Blob: blob, Meta: meta}
	if err := uc.Execute(ctx, share); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(blob.DeleteCalls) != 1 || blob.DeleteCalls[0] != "7-id1234567890" {
		t.Errorf("blob delete call wrong: %v", blob.DeleteCalls)
	}
	if len(meta.RemoveCleanupCalls) != 1 || meta.RemoveCleanupCalls[0] != "7-id1234567890" {
		t.Errorf("RemoveCleanup call wrong: %v", meta.RemoveCleanupCalls)
	}
	if len(meta.DeleteCalls) != 1 || meta.DeleteCalls[0] != share.ID {
		t.Errorf("meta delete call wrong: %v", meta.DeleteCalls)
	}
}

func TestDeleteShare_BlobErrorAborts(t *testing.T) {
	blob := portfakes.NewFakeBlobStorage()
	blob.ErrOnDelete = errors.New("s3 down")
	meta := portfakes.NewFakeMetaStore()
	uc := &usecase.DeleteShare{Blob: blob, Meta: meta}

	share := &domain.FileShare{ID: "id1234567890", Prefix: 1}
	err := uc.Execute(context.Background(), share)
	if !errors.Is(err, blob.ErrOnDelete) {
		t.Fatalf("expected blob delete err to bubble up, got %v", err)
	}
	if len(meta.RemoveCleanupCalls) != 0 || len(meta.DeleteCalls) != 0 {
		t.Errorf("meta should not be touched on blob error: rm=%v del=%v", meta.RemoveCleanupCalls, meta.DeleteCalls)
	}
}
