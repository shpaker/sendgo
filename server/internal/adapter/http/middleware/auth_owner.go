package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/sendgo/sendgo/server/internal/adapter/observability"
	"github.com/sendgo/sendgo/server/internal/crypto"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// OwnerDeps lists what the owner middleware needs.
type OwnerDeps struct {
	Meta port.MetaStore
}

// Owner implements owner-auth (port of server/middleware/auth.js:owner).
// Reads the JSON body, extracts owner_token, compares it to share.Owner.
// On success the share is placed in context.
//
// NOTE: the middleware reads the body once and replaces r.Body with a
// bytes.Buffer so the handler can re-read it (the body may also carry
// dlimit, auth, etc.).
func Owner(deps OwnerDeps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			if !domain.ValidShareID(idStr) {
				http.Error(w, "invalid id", http.StatusBadRequest)
				return
			}
			id := domain.ShareID(idStr)
			lg := observability.FromContext(r.Context())

			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
			if err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			// Restore the body so the handler can re-parse it (dlimit, auth, etc. live there too).
			r.Body = io.NopCloser(bytes.NewReader(body))

			var payload struct {
				OwnerToken string `json:"owner_token"`
			}
			if err := json.Unmarshal(body, &payload); err != nil || payload.OwnerToken == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			share, err := deps.Meta.Get(r.Context(), id)
			if errors.Is(err, domain.ErrNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			if err != nil {
				lg.Error("owner middleware: meta.Get failed", "err", err)
				http.Error(w, "internal", http.StatusInternalServerError)
				return
			}

			if !crypto.CompareTokens(string(share.Owner), payload.OwnerToken) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := WithShare(r.Context(), share)
			ctx = observability.WithLogger(ctx, lg.With("share_id", string(id)))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
