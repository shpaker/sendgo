package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port/portfakes"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

func TestUpdateParams_HappyPath(t *testing.T) {
	ctx := context.Background()
	meta := portfakes.NewFakeMetaStore()
	uc := &usecase.UpdateParams{Meta: meta, MaxDownloads: 100}

	share := &domain.FileShare{ID: "id1234567890", DLimit: 1, DL: 0}
	if err := uc.Execute(ctx, share, 5); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(meta.SetFieldCalls) != 1 || meta.SetFieldCalls[0].Field != "dlimit" || meta.SetFieldCalls[0].Value != "5" {
		t.Errorf("unexpected SetField call: %+v", meta.SetFieldCalls)
	}
}

func TestUpdateParams_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	uc := &usecase.UpdateParams{MaxDownloads: 10}
	share := &domain.FileShare{ID: "id1234567890", DLimit: 1, DL: 3}

	cases := []struct {
		name      string
		newDLimit int
	}{
		{"below 1", 0},
		{"above max", 11},
		{"≤ already downloaded", 3},
		{"equal to downloaded", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := uc.Execute(ctx, share, c.newDLimit)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, domain.ErrBadRequest) {
				t.Errorf("want ErrBadRequest, got %v", err)
			}
		})
	}
}

func TestUpdateParams_SetFieldError(t *testing.T) {
	meta := portfakes.NewFakeMetaStore()
	meta.ErrOnSetField = errors.New("boom")
	uc := &usecase.UpdateParams{Meta: meta, MaxDownloads: 10}

	err := uc.Execute(context.Background(), &domain.FileShare{ID: "x", DL: 0}, 5)
	if err == nil {
		t.Fatal("expected SetField error to propagate")
	}
}
