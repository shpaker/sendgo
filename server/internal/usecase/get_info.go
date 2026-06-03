package usecase

import (
	"context"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// GetInfo serves POST /api/info/:id. Owner-only view: limits + downloads so
// far + TTL.
type GetInfo struct {
	Meta port.MetaStore
}

func (uc *GetInfo) Execute(ctx context.Context, share *domain.FileShare) (*ShareInfoOutput, error) {
	ttl, err := uc.Meta.TTL(ctx, share.ID)
	if err != nil {
		return nil, err
	}
	return &ShareInfoOutput{
		DLimit: share.DLimit,
		DL:     share.DL,
		TTL:    ttl,
	}, nil
}
