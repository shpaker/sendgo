package usecase

import (
	"context"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// DeleteShare serves POST /api/delete/:id. The owner middleware has already
// validated the token and injected the share into the context, so the use
// case simply removes three things: blob → cleanup index → meta. Order: blob
// first (if any) to avoid orphans.
type DeleteShare struct {
	Blob port.BlobStorage
	Meta port.MetaStore
}

func (uc *DeleteShare) Execute(ctx context.Context, share *domain.FileShare) error {
	key := blobKey(share)
	if err := uc.Blob.Delete(ctx, key); err != nil {
		return err
	}
	_ = uc.Meta.RemoveCleanup(ctx, key)
	return uc.Meta.Delete(ctx, share.ID)
}
