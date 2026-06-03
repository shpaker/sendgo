package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/getsentry/sentry-go"

	"github.com/sendgo/sendgo/server/internal/adapter/observability"
)

// Recovery catches panics, logs them, reports to Sentry and responds with 500.
// Should sit near the outermost layer of middleware (right after RequestID).
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				lg := observability.FromContext(r.Context())
				lg.Error("panic recovered",
					"err", fmt.Sprint(rec),
					"stack", string(debug.Stack()),
					"path", r.URL.Path,
				)
				sentry.CurrentHub().Recover(rec)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
