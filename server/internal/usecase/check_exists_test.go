package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port/portfakes"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

func TestCheckExists_Execute(t *testing.T) {
	ctx := context.Background()
	storedNoPwd := &domain.FileShare{ID: "id1234567890", Password: false}
	storedWithPwd := &domain.FileShare{ID: "id1234567890", Password: true}

	cases := []struct {
		name        string
		seed        *domain.FileShare
		errOnGet    error
		wantExists  bool
		wantNeedPwd bool
		wantErr     bool
	}{
		{name: "missing", seed: nil, wantExists: false},
		{name: "exists no password", seed: storedNoPwd, wantExists: true, wantNeedPwd: false},
		{name: "exists with password", seed: storedWithPwd, wantExists: true, wantNeedPwd: true},
		{name: "unexpected store error", seed: nil, errOnGet: errors.New("boom"), wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			meta := portfakes.NewFakeMetaStore()
			if c.seed != nil {
				_ = meta.Set(ctx, c.seed, 60)
			}
			meta.ErrOnGet = c.errOnGet
			uc := &usecase.CheckExists{Meta: meta}

			out, err := uc.Execute(ctx, "id1234567890")
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if out.Exists != c.wantExists || out.RequiresPassword != c.wantNeedPwd {
				t.Errorf("out=%+v want exists=%v needPwd=%v", out, c.wantExists, c.wantNeedPwd)
			}
		})
	}
}
