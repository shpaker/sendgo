package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/sendgo/sendgo/server/internal/adapter/http/middleware"
	"github.com/sendgo/sendgo/server/internal/adapter/observability"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

// Download serves GET /api/download/:id and /api/download/blob/:id (used
// when there is no service worker and the file is downloaded as a blob).
// HMAC is already verified and the nonce already rotated by the middleware.
// Contract is 1:1 with server/routes/download.js — octet-stream + counter
// increment after successful delivery.
type Download struct{ UC *usecase.StreamDownload }

func (h *Download) Handle(w http.ResponseWriter, r *http.Request) {
	share := middleware.ShareFromContext(r.Context())
	if share == nil {
		WriteError(w, r, domain.ErrUnauthorized)
		return
	}
	lg := observability.FromContext(r.Context())

	rc, err := h.UC.Open(r.Context(), share)
	if errors.Is(err, domain.ErrNotFound) {
		WriteError(w, r, err)
		return
	}
	if err != nil {
		WriteError(w, r, err)
		return
	}
	defer func() { _ = rc.Close() }()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	// io.Copy without a buffer — stream the chunks as S3/FS yield them.
	if _, err := io.Copy(w, rc); err != nil {
		// The client may close the connection — that's expected; treat it as
		// "did not finish delivering" and skip the counter increment (matches
		// Node's pipe.on('finish') semantics).
		lg.Info("download copy aborted", "err", err)
		return
	}

	if err := h.UC.Finalize(r.Context(), share); err != nil {
		lg.Error("download finalize failed", "err", err)
	}
}
