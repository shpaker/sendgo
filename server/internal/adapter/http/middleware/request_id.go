// Package middleware contains HTTP middleware for sendgo.
//
// Architecturally placed next to handlers/, rather than directly in http/,
// so router.go can wire them up declaratively.
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/sendgo/sendgo/server/internal/adapter/observability"
)

type ctxKey struct{ k string }

var requestIDKey = ctxKey{"request_id"}

// RequestID puts a correlation id into the context and response header on
// the way in. If the client sent X-Request-ID, we use it; otherwise we
// generate a uuid v4. The logger from context is enriched with a request_id
// field.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		lg := observability.FromContext(ctx).With("request_id", id)
		ctx = observability.WithLogger(ctx, lg)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext extracts the id for tests / additional logging.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}
