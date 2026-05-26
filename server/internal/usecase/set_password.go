package usecase

import (
	"context"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// SetPassword serves POST /api/password/:id. The client sends a new `auth`
// value (base64-encoded HMAC key derived from the password via PBKDF2 on the
// client). We just persist it and flip pwd=true. Matches
// server/routes/password.js.
type SetPassword struct {
	Meta port.MetaStore
}

func (uc *SetPassword) Execute(ctx context.Context, share *domain.FileShare, authB64 string) error {
	if err := uc.Meta.SetField(ctx, share.ID, "auth", authB64); err != nil {
		return err
	}
	return uc.Meta.SetField(ctx, share.ID, "pwd", "true")
}
