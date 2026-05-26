package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port/portfakes"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

func validInput() usecase.InitiateUploadInput {
	return usecase.InitiateUploadInput{
		FileMetadata: []byte("meta"),
		AuthKey:      []byte("auth"),
		TimeLimit:    3600,
		DLimit:       5,
		BaseURL:      "https://send.example/",
	}
}

func newInitiateUpload(tg *portfakes.FakeTokenGenerator) *usecase.InitiateUpload {
	return &usecase.InitiateUpload{
		Tokens:   tg,
		MaxFile:  1 << 30,
		MaxTTL:   604800,
		MaxDL:    100,
		Defaults: usecase.Defaults{ExpireSeconds: 86400, Downloads: 1},
	}
}

func TestInitiateUpload_HappyPath(t *testing.T) {
	tg := portfakes.NewFakeTokenGenerator(
		[]domain.ShareID{"shareid000000001"},
		[]domain.OwnerToken{"ownertoken0000000002"},
		[][]byte{{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3, 4, 5, 6}},
	)
	uc := newInitiateUpload(tg)

	out, err := uc.Execute(context.Background(), validInput())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.ID != "shareid000000001" || out.OwnerToken != "ownertoken0000000002" {
		t.Errorf("ID/owner: %+v", out)
	}
	// BaseURL trailing slash must be trimmed.
	if out.URL != "https://send.example/download/shareid000000001/" {
		t.Errorf("URL=%q", out.URL)
	}
	// Prefix = DayPrefix(3600) = max(3600/86400, 1) = 1.
	if out.S3Key != "1-shareid000000001" {
		t.Errorf("S3Key=%q", out.S3Key)
	}
	if out.Share.DLimit != 5 || out.Share.ExpireSeconds != 3600 {
		t.Errorf("Share defaults wrong: %+v", out.Share)
	}
	if string(out.Share.EncryptedMetadata) != "meta" {
		t.Errorf("EncryptedMetadata not copied through: %q", out.Share.EncryptedMetadata)
	}
}

func TestInitiateUpload_DefaultsApplied(t *testing.T) {
	tg := portfakes.NewFakeTokenGenerator(nil, nil, nil)
	uc := newInitiateUpload(tg)
	in := validInput()
	in.TimeLimit = 0
	in.DLimit = 0

	out, err := uc.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out.Share.ExpireSeconds != 86400 || out.Share.DLimit != 1 {
		t.Errorf("defaults not applied: %+v", out.Share)
	}
}

func TestInitiateUpload_ValidationErrors(t *testing.T) {
	mutate := map[string]func(*usecase.InitiateUploadInput){
		"empty metadata":  func(in *usecase.InitiateUploadInput) { in.FileMetadata = nil },
		"empty auth key":  func(in *usecase.InitiateUploadInput) { in.AuthKey = nil },
		"timeLimit > max": func(in *usecase.InitiateUploadInput) { in.TimeLimit = 1_000_000_000 },
		"dlimit > max":    func(in *usecase.InitiateUploadInput) { in.DLimit = 999 },
		"baseURL empty":   func(in *usecase.InitiateUploadInput) { in.BaseURL = "" },
	}
	for name, m := range mutate {
		t.Run(name, func(t *testing.T) {
			tg := portfakes.NewFakeTokenGenerator(nil, nil, nil)
			uc := newInitiateUpload(tg)
			in := validInput()
			m(&in)
			_, err := uc.Execute(context.Background(), in)
			if !errors.Is(err, domain.ErrBadRequest) {
				t.Fatalf("err=%v want ErrBadRequest", err)
			}
			if tg.ShareIDCalls != 0 {
				t.Errorf("token generator should not have been called after validation failure")
			}
		})
	}
}

func TestInitiateUpload_DefaultsZeroThenTimeLimitFails(t *testing.T) {
	tg := portfakes.NewFakeTokenGenerator(nil, nil, nil)
	uc := newInitiateUpload(tg)
	uc.Defaults.ExpireSeconds = 0 // default itself is zero
	in := validInput()
	in.TimeLimit = 0 // falls into default → 0 → fails the timeLimit<=0 branch

	_, err := uc.Execute(context.Background(), in)
	if !errors.Is(err, domain.ErrBadRequest) {
		t.Fatalf("err=%v want ErrBadRequest", err)
	}
}

func TestInitiateUpload_DefaultsZeroThenDLimitFails(t *testing.T) {
	tg := portfakes.NewFakeTokenGenerator(nil, nil, nil)
	uc := newInitiateUpload(tg)
	uc.Defaults.Downloads = 0
	in := validInput()
	in.DLimit = 0 // → default 0 → fails dlimit < 1

	_, err := uc.Execute(context.Background(), in)
	if !errors.Is(err, domain.ErrBadRequest) {
		t.Fatalf("err=%v want ErrBadRequest", err)
	}
}

func TestInitiateUpload_TokenErrors(t *testing.T) {
	cases := []struct {
		name string
		set  func(*portfakes.FakeTokenGenerator)
	}{
		{"share id", func(tg *portfakes.FakeTokenGenerator) { tg.ErrOnShareID = errors.New("id") }},
		{"owner token", func(tg *portfakes.FakeTokenGenerator) { tg.ErrOnOwnerToken = errors.New("ow") }},
		{"nonce", func(tg *portfakes.FakeTokenGenerator) { tg.ErrOnNonce = errors.New("no") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tg := portfakes.NewFakeTokenGenerator(nil, nil, nil)
			c.set(tg)
			uc := newInitiateUpload(tg)
			_, err := uc.Execute(context.Background(), validInput())
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
