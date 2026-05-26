package usecase

import (
	"context"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// GetMetadata serves GET /api/metadata/:id. Invoked after the HMAC middleware
// (which loaded the share into context). Here we only query the TTL and
// compute the finalDownload flag.
type GetMetadata struct {
	Meta port.MetaStore
}

// Execute takes an already-loaded share (the auth middleware read and HMAC-
// verified it) and returns data for the JSON response.
func (uc *GetMetadata) Execute(ctx context.Context, share *domain.FileShare) (*MetadataOutput, error) {
	ttl, err := uc.Meta.TTL(ctx, share.ID)
	if err != nil {
		return nil, err
	}
	return &MetadataOutput{
		EncryptedMetadata: share.EncryptedMetadata,
		FinalDownload:     share.IsFinalDownload(),
		TTL:               ttl,
	}, nil
}
