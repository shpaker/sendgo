package usecase_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port/portfakes"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

func TestStreamDownload_OpenHappyPath(t *testing.T) {
	ctx := context.Background()
	blob := portfakes.NewFakeBlobStorage()
	meta := portfakes.NewFakeMetaStore()
	share := &domain.FileShare{ID: "id1234567890", Prefix: 3, DLimit: 5}
	_ = meta.Set(ctx, share, 60)
	_ = blob.Put(ctx, "3-id1234567890", bytes.NewReader([]byte("payload")), 0)

	uc := &usecase.StreamDownload{Blob: blob, Meta: meta}
	rc, err := uc.Open(ctx, share)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, _ := io.ReadAll(rc)
	if string(got) != "payload" {
		t.Errorf("body=%q want payload", got)
	}
	// Meta must not be touched on the happy path.
	if len(meta.DeleteCalls) != 0 || len(meta.RemoveCleanupCalls) != 0 {
		t.Errorf("meta was touched: delete=%v rm=%v", meta.DeleteCalls, meta.RemoveCleanupCalls)
	}
}

func TestStreamDownload_OpenNotFoundCleansUp(t *testing.T) {
	ctx := context.Background()
	blob := portfakes.NewFakeBlobStorage()
	meta := portfakes.NewFakeMetaStore()
	share := &domain.FileShare{ID: "id1234567890", Prefix: 3}
	_ = meta.Set(ctx, share, 60)

	uc := &usecase.StreamDownload{Blob: blob, Meta: meta}
	_, err := uc.Open(ctx, share)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Open: %v, want ErrNotFound", err)
	}
	if len(meta.DeleteCalls) != 1 || meta.DeleteCalls[0] != share.ID {
		t.Errorf("expected meta delete on missing blob, got %v", meta.DeleteCalls)
	}
	if len(meta.RemoveCleanupCalls) != 1 || meta.RemoveCleanupCalls[0] != "3-id1234567890" {
		t.Errorf("expected RemoveCleanup on missing blob, got %v", meta.RemoveCleanupCalls)
	}
}

func TestStreamDownload_OpenOtherError(t *testing.T) {
	blob := portfakes.NewFakeBlobStorage()
	blob.ErrOnGet = errors.New("s3 down")
	uc := &usecase.StreamDownload{Blob: blob, Meta: portfakes.NewFakeMetaStore()}

	_, err := uc.Open(context.Background(), &domain.FileShare{ID: "x", Prefix: 1})
	if err == nil || errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Open: %v, want non-NotFound transport error", err)
	}
}

func TestStreamDownload_FinalizeUnderLimit(t *testing.T) {
	ctx := context.Background()
	meta := portfakes.NewFakeMetaStore()
	share := &domain.FileShare{ID: "id1234567890", DLimit: 3, DL: 0}
	_ = meta.Set(ctx, share, 60)

	uc := &usecase.StreamDownload{Blob: portfakes.NewFakeBlobStorage(), Meta: meta}
	if err := uc.Finalize(ctx, share); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if len(meta.DeleteCalls) != 0 {
		t.Errorf("meta.Delete called early: %v", meta.DeleteCalls)
	}
}

func TestStreamDownload_FinalizeAtLimitCleansUp(t *testing.T) {
	ctx := context.Background()
	blob := portfakes.NewFakeBlobStorage()
	meta := portfakes.NewFakeMetaStore()
	share := &domain.FileShare{ID: "id1234567890", Prefix: 2, DLimit: 1, DL: 0}
	_ = meta.Set(ctx, share, 60)
	_ = blob.Put(ctx, "2-id1234567890", bytes.NewReader([]byte("x")), 0)

	uc := &usecase.StreamDownload{Blob: blob, Meta: meta}
	if err := uc.Finalize(ctx, share); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if len(blob.DeleteCalls) != 1 || blob.DeleteCalls[0] != "2-id1234567890" {
		t.Errorf("blob delete: %v", blob.DeleteCalls)
	}
	if len(meta.DeleteCalls) != 1 || meta.DeleteCalls[0] != share.ID {
		t.Errorf("meta delete: %v", meta.DeleteCalls)
	}
	if len(meta.RemoveCleanupCalls) != 1 || meta.RemoveCleanupCalls[0] != "2-id1234567890" {
		t.Errorf("RemoveCleanup: %v", meta.RemoveCleanupCalls)
	}
}

func TestStreamDownload_FinalizeIncrError(t *testing.T) {
	meta := portfakes.NewFakeMetaStore()
	meta.ErrOnIncrField = errors.New("incr down")
	uc := &usecase.StreamDownload{Blob: portfakes.NewFakeBlobStorage(), Meta: meta}

	err := uc.Finalize(context.Background(), &domain.FileShare{ID: "x", DLimit: 5})
	if err == nil {
		t.Fatal("expected error")
	}
}
