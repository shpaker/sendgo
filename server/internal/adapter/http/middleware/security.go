package middleware

import "net/http"

// SecurityHeaders is a port of `helmet()` from the current Node server. We
// don't try to mirror every CSP directive (those depended on FXA config,
// which we no longer have); instead we set sensible defaults for static + API.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "interest-cohort=()")
		// CSP for the SPA. Service Worker is required for downloads (see app/),
		// so 'self' for script-src/worker-src is mandatory.
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'wasm-unsafe-eval'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; "+
				"connect-src 'self' ws: wss:; "+
				"worker-src 'self' blob:; "+
				"frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}
