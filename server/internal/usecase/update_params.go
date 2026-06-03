package usecase

import (
	"context"
	"fmt"
	"strconv"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// UpdateParams serves POST /api/params/:id. We change only dlimit (matching
// Node). Validation: dlimit > 0, dlimit ≤ MaxDownloads, dlimit > already
// downloaded.
type UpdateParams struct {
	Meta         port.MetaStore
	MaxDownloads int
}

func (uc *UpdateParams) Execute(ctx context.Context, share *domain.FileShare, newDLimit int) error {
	if newDLimit < 1 || newDLimit > uc.MaxDownloads {
		return fmt.Errorf("%w: dlimit %d out of range [1, %d]", domain.ErrBadRequest, newDLimit, uc.MaxDownloads)
	}
	if newDLimit <= share.DL {
		return fmt.Errorf("%w: dlimit %d ≤ already downloaded %d", domain.ErrBadRequest, newDLimit, share.DL)
	}
	return uc.Meta.SetField(ctx, share.ID, "dlimit", strconv.Itoa(newDLimit))
}
