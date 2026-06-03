package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/sendgo/sendgo/server/internal/adapter/observability"
	"github.com/sendgo/sendgo/server/internal/crypto"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port"
)

// loadedShareCtxKey is a dedicated type to avoid context-key collisions.
type loadedShareCtxKey struct{}

// WithShare stores the loaded share in context (used by middleware to pass
// it through to the handler).
func WithShare(ctx context.Context, s *domain.FileShare) context.Context {
	return context.WithValue(ctx, loadedShareCtxKey{}, s)
}

// ShareFromContext retrieves the share inside a handler. Returns nil when
// the middleware did not run.
func ShareFromContext(ctx context.Context) *domain.FileShare {
	if v, ok := ctx.Value(loadedShareCtxKey{}).(*domain.FileShare); ok {
		return v
	}
	return nil
}

// HMACDeps lists what the middleware needs to operate. Supplied when the
// router is wired up.
type HMACDeps struct {
	Meta   port.MetaStore
	Tokens port.TokenGenerator
}

// HMAC implements HMAC + nonce-challenge auth (port of
// server/middleware/auth.js). Algorithm:
//  1. Read `id` from the path and `Authorization: send-v1 <b64hmac>` from the header.
//  2. Load the share from meta. nil → 404.
//  3. Compare HMAC-SHA256(share.AuthKey, share.Nonce) against the supplied signature.
//  4. On match: generate a new nonce, SetField(nonce, ...), set
//     WWW-Authenticate, call next.
//  5. On mismatch: set WWW-Authenticate with the **old** nonce, return 401.
func HMAC(deps HMACDeps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			idStr := chi.URLParam(r, "id")
			if !domain.ValidShareID(idStr) {
				http.Error(w, "invalid id", http.StatusBadRequest)
				return
			}
			id := domain.ShareID(idStr)
			lg := observability.FromContext(r.Context())

			share, err := deps.Meta.Get(r.Context(), id)
			if errors.Is(err, domain.ErrNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			if err != nil {
				lg.Error("hmac middleware: meta.Get failed", "err", err)
				http.Error(w, "internal", http.StatusInternalServerError)
				return
			}

			auth := r.Header.Get("Authorization")
			parts := strings.SplitN(auth, " ", 2)
			if len(parts) != 2 || parts[0] != "send-v1" {
				// The client must send send-v1 <b64>. Hand them the current nonce.
				w.Header().Set("WWW-Authenticate", "send-v1 "+crypto.EncodeB64(share.Nonce))
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			// The frontend sends URL-safe without padding — crypto.DecodeB64 accepts any flavor.
			supplied, err := crypto.DecodeB64(parts[1])
			if err != nil {
				w.Header().Set("WWW-Authenticate", "send-v1 "+crypto.EncodeB64(share.Nonce))
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			if !crypto.VerifyHMAC(share.AuthKey, share.Nonce, supplied) {
				w.Header().Set("WWW-Authenticate", "send-v1 "+crypto.EncodeB64(share.Nonce))
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// Success: rotate the nonce.
			newNonce, err := deps.Tokens.Nonce()
			if err != nil {
				lg.Error("hmac middleware: nonce gen failed", "err", err)
				http.Error(w, "internal", http.StatusInternalServerError)
				return
			}
			newNonceB64 := crypto.EncodeB64(newNonce)
			if err := deps.Meta.SetField(r.Context(), id, "nonce", newNonceB64); err != nil {
				lg.Error("hmac middleware: nonce persist failed", "err", err)
				http.Error(w, "internal", http.StatusInternalServerError)
				return
			}
			share.Nonce = newNonce
			w.Header().Set("WWW-Authenticate", "send-v1 "+newNonceB64)

			ctx := WithShare(r.Context(), share)
			ctx = observability.WithLogger(ctx, lg.With("share_id", string(id)))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
