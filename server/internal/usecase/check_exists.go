package usecase

import (
	"context"
	"errors"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// CheckExists serves GET /api/exists/:id. A lightweight read: returns only
// flags, no metadata. The `{password: bool}` response contract is enforced
// in the HTTP handler.
type CheckExists struct {
	Meta port.MetaStore
}

func (uc *CheckExists) Execute(ctx context.Context, id domain.ShareID) (*ExistsOutput, error) {
	s, err := uc.Meta.Get(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return &ExistsOutput{Exists: false}, nil
	}
	if err != nil {
		return nil, err
	}
	return &ExistsOutput{Exists: true, RequiresPassword: s.Password}, nil
}
