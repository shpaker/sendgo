package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port/portfakes"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

func TestSetPassword_HappyPath(t *testing.T) {
	ctx := context.Background()
	meta := portfakes.NewFakeMetaStore()
	uc := &usecase.SetPassword{Meta: meta}

	share := &domain.FileShare{ID: "id1234567890"}
	if err := uc.Execute(ctx, share, "newauth=="); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(meta.SetFieldCalls) != 2 {
		t.Fatalf("want 2 SetField calls, got %d: %+v", len(meta.SetFieldCalls), meta.SetFieldCalls)
	}
	if meta.SetFieldCalls[0].Field != "auth" || meta.SetFieldCalls[0].Value != "newauth==" {
		t.Errorf("first call: %+v", meta.SetFieldCalls[0])
	}
	if meta.SetFieldCalls[1].Field != "pwd" || meta.SetFieldCalls[1].Value != "true" {
		t.Errorf("second call: %+v", meta.SetFieldCalls[1])
	}
}

func TestSetPassword_FirstSetFieldFails(t *testing.T) {
	ctx := context.Background()
	meta := portfakes.NewFakeMetaStore()
	meta.ErrOnSetField = errors.New("boom")
	uc := &usecase.SetPassword{Meta: meta}

	err := uc.Execute(ctx, &domain.FileShare{ID: "x"}, "auth")
	if err == nil {
		t.Fatal("expected error from first SetField")
	}
	// Even on error the fake records the call. Both calls happen because
	// FakeMetaStore.SetField always returns the same ErrOnSetField — but the
	// use case stops after the first failure. Verify only one SetField call.
	if len(meta.SetFieldCalls) != 1 {
		t.Errorf("expected exactly 1 SetField call before bail-out, got %d", len(meta.SetFieldCalls))
	}
}
