package handlers

import (
	"net/http"

	"github.com/sendgo/sendgo/server/internal/adapter/http/middleware"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

// Info serves POST /api/info/:id. Body: `{owner_token}` (port of
// server/routes/info.js). Response: `{dlimit, dtotal, ttl}` (ttl in
// milliseconds).
type Info struct{ UC *usecase.GetInfo }

func (h *Info) Handle(w http.ResponseWriter, r *http.Request) {
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
		"dlimit": out.DLimit,
		"dtotal": out.DL,
		"ttl":    out.TTL.Milliseconds(),
	})
}
