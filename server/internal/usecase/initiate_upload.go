package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// InitiateUpload — first phase of the WS upload: validate input, generate
// id/owner/nonce, derive the S3 key. The meta-store write happens later, in
// StreamUpload, AFTER the blob has been successfully written (blob-then-meta
// ordering, so a crash never leaves a meta row without a blob).
type InitiateUpload struct {
	Tokens   port.TokenGenerator
	MaxFile  int64 // CLI.MaxFileSize
	MaxTTL   int
	MaxDL    int
	Defaults Defaults
}

// Defaults — CLI defaults pulled out into a small struct so the use case
// doesn't import *config.CLI.
type Defaults struct {
	ExpireSeconds int
	Downloads     int
}

func (uc *InitiateUpload) Execute(ctx context.Context, in InitiateUploadInput) (*InitiateUploadOutput, error) {
	_ = ctx
	// Apply defaults (matches server/routes/ws.js behavior).
	timeLimit := in.TimeLimit
	if timeLimit == 0 {
		timeLimit = uc.Defaults.ExpireSeconds
	}
	dlimit := in.DLimit
	if dlimit == 0 {
		dlimit = uc.Defaults.Downloads
	}

	// Validation (1:1 with server/routes/ws.js conditions).
	if len(in.FileMetadata) == 0 || len(in.AuthKey) == 0 {
		return nil, fmt.Errorf("%w: metadata and authorization are required", domain.ErrBadRequest)
	}
	if timeLimit <= 0 || timeLimit > uc.MaxTTL {
		return nil, fmt.Errorf("%w: timeLimit %d not in (0, %d]", domain.ErrBadRequest, timeLimit, uc.MaxTTL)
	}
	if dlimit < 1 || dlimit > uc.MaxDL {
		return nil, fmt.Errorf("%w: dlimit %d not in [1, %d]", domain.ErrBadRequest, dlimit, uc.MaxDL)
	}
	if in.BaseURL == "" {
		return nil, fmt.Errorf("%w: base URL is required", domain.ErrBadRequest)
	}

	id, err := uc.Tokens.ShareID()
	if err != nil {
		return nil, err
	}
	owner, err := uc.Tokens.OwnerToken()
	if err != nil {
		return nil, err
	}
	nonce, err := uc.Tokens.Nonce()
	if err != nil {
		return nil, err
	}

	prefix := domain.DayPrefix(timeLimit)
	share := &domain.FileShare{
		ID:                id,
		Owner:             owner,
		EncryptedMetadata: in.FileMetadata,
		AuthKey:           in.AuthKey,
		Nonce:             nonce,
		Password:          false,
		DLimit:            dlimit,
		DL:                0,
		ExpireSeconds:     timeLimit,
		Prefix:            prefix,
	}

	base := strings.TrimRight(in.BaseURL, "/")
	return &InitiateUploadOutput{
		ID:         id,
		OwnerToken: owner,
		URL:        fmt.Sprintf("%s/download/%s/", base, id),
		S3Key:      fmt.Sprintf("%d-%s", prefix, id),
		Share:      share,
	}, nil
}
