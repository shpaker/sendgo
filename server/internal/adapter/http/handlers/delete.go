package handlers

import (
	"net/http"

	"github.com/sendgo/sendgo/server/internal/adapter/http/middleware"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

// Delete serves POST /api/delete/:id. The owner middleware has already
// validated the token.
type Delete struct{ UC *usecase.DeleteShare }

func (h *Delete) Handle(w http.ResponseWriter, r *http.Request) {
	share := middleware.ShareFromContext(r.Context())
	if share == nil {
		WriteError(w, r, domain.ErrUnauthorized)
		return
	}
	if err := h.UC.Execute(r.Context(), share); err != nil {
		WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
