// Package handlers contains the concrete sendgo HTTP endpoints. Each file is
// one route or family of routes. All handlers are methods on receiver
// structs, so the router can inject use cases once at wiring time.
package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

// Exists serves GET /api/exists/:id. Response body `{password: bool}` — port
// of server/routes/exists.js.
type Exists struct{ UC *usecase.CheckExists }

func (h *Exists) Handle(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	if !domain.ValidShareID(idStr) {
		WriteError(w, r, domain.ErrInvalidShareID)
		return
	}
	out, err := h.UC.Execute(r.Context(), domain.ShareID(idStr))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if !out.Exists {
		WriteError(w, r, domain.ErrNotFound)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]bool{"password": out.RequiresPassword})
}
