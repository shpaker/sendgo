package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/sendgo/sendgo/server/internal/adapter/http/middleware"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

// Password serves POST /api/password/:id. Body: `{owner_token, auth}` (port
// of server/routes/password.js). Persists the new auth (base64 from the
// client) and flips pwd=true.
type Password struct{ UC *usecase.SetPassword }

func (h *Password) Handle(w http.ResponseWriter, r *http.Request) {
	share := middleware.ShareFromContext(r.Context())
	if share == nil {
		WriteError(w, r, domain.ErrUnauthorized)
		return
	}
	var payload struct {
		Auth string `json:"auth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Auth == "" {
		WriteError(w, r, domain.ErrBadRequest)
		return
	}
	if err := h.UC.Execute(r.Context(), share, payload.Auth); err != nil {
		WriteError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
