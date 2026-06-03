package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port/portfakes"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

func TestGetMetadata_Execute(t *testing.T) {
	ctx := context.Background()
	meta := portfakes.NewFakeMetaStore()
	share := &domain.FileShare{
		ID:                "id1234567890",
		EncryptedMetadata: []byte("ciphertext"),
		DLimit:            3,
		DL:                1,
	}
	_ = meta.Set(ctx, share, 0)
	meta.TTLs[share.ID] = 90 * time.Second

	uc := &usecase.GetMetadata{Meta: meta}

	out, err := uc.Execute(ctx, share)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if string(out.EncryptedMetadata) != "ciphertext" || out.TTL != 90*time.Second || out.FinalDownload {
		t.Errorf("out=%+v", out)
	}

	// Bump DL so DL+1 == DLimit → FinalDownload should flip.
	share.DL = 2
	out, _ = uc.Execute(ctx, share)
	if !out.FinalDownload {
		t.Errorf("expected FinalDownload=true when DL+1 == DLimit")
	}
}

func TestGetMetadata_TTLError(t *testing.T) {
	meta := portfakes.NewFakeMetaStore()
	meta.ErrOnTTL = errors.New("boom")
	uc := &usecase.GetMetadata{Meta: meta}

	_, err := uc.Execute(context.Background(), &domain.FileShare{ID: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}
