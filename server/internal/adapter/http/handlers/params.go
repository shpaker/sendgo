package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/sendgo/sendgo/server/internal/adapter/http/middleware"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

// Params serves POST /api/params/:id. Body: `{owner_token, dlimit}` (port
// of server/routes/params.js).
type Params struct{ UC *usecase.UpdateParams }

func (h *Params) Handle(w http.ResponseWriter, r *http.Request) {
	share := middleware.ShareFromContext(r.Context())
	if share == nil {
		WriteError(w, r, domain.ErrUnauthorized)
		return
	}
	var payload struct {
		DLimit int `json:"dlimit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		WriteError(w, r, domain.ErrBadRequest)
		return
	}
	if err := h.UC.Execute(r.Context(), share, payload.DLimit); err != nil {
		WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
