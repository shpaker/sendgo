package handlers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/sendgo/sendgo/server/internal/adapter/observability"
	"github.com/sendgo/sendgo/server/internal/domain"
)

// WriteJSON renders the given value as JSON with the specified status code.
func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError maps a domain error to an HTTP status. No RFC 7807 — contract
// is 1:1 with Node. The response body is plain text/plain, matching
// Express's default error handler.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	code := codeFromError(err)
	if code >= 500 {
		lg := observability.FromContext(r.Context())
		lg.Error("handler error",
			slog.String("path", r.URL.Path),
			slog.Int("status", code),
			slog.Any("err", err),
		)
	}
	http.Error(w, http.StatusText(code), code)
}

func codeFromError(err error) int {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, domain.ErrInvalidAuth):
		return http.StatusUnauthorized
	case errors.Is(err, domain.ErrBadRequest), errors.Is(err, domain.ErrInvalidShareID):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrPayloadTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, domain.ErrLimitReached):
		return http.StatusGone
	default:
		return http.StatusInternalServerError
	}
}
