package handlers

import (
	"net/http"

	"github.com/sendgo/sendgo/server/internal/adapter/http/middleware"
	"github.com/sendgo/sendgo/server/internal/crypto"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

// Metadata serves GET /api/metadata/:id. The HMAC middleware has already
// run and placed the share in context. Response:
// `{metadata: <b64>, finalDownload: bool, ttl: <ms>}` (port of
// server/routes/metadata.js).
type Metadata struct{ UC *usecase.GetMetadata }

func (h *Metadata) Handle(w http.ResponseWriter, r *http.Request) {
	share := middleware.ShareFromContext(r.Context())
	if share == nil {
		WriteError(w, r, domain.ErrUnauthorized)
		return
	}
	out, err := h.UC.Execute(r.Context(), share)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"metadata":      crypto.EncodeB64(out.EncryptedMetadata),
		"finalDownload": out.FinalDownload,
		"ttl":           out.TTL.Milliseconds(),
	})
}
