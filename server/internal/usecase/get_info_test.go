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

func TestGetInfo_Execute(t *testing.T) {
	ctx := context.Background()
	meta := portfakes.NewFakeMetaStore()
	share := &domain.FileShare{ID: "id1234567890", DLimit: 10, DL: 4}
	_ = meta.Set(ctx, share, 0)
	meta.TTLs[share.ID] = 120 * time.Second

	uc := &usecase.GetInfo{Meta: meta}
	out, err := uc.Execute(ctx, share)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.DLimit != 10 || out.DL != 4 || out.TTL != 120*time.Second {
		t.Errorf("out=%+v", out)
	}
}

func TestGetInfo_TTLError(t *testing.T) {
	meta := portfakes.NewFakeMetaStore()
	meta.ErrOnTTL = errors.New("boom")
	uc := &usecase.GetInfo{Meta: meta}

	_, err := uc.Execute(context.Background(), &domain.FileShare{ID: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}
