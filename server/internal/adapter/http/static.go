package http

import (
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// StaticHandler returns an http.Handler that serves static assets from an
// embed.FS under the given prefix. For digested names (main.<hash>.js) it
// sets a long Cache-Control.
func StaticHandler(prefix string, fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	stripped := http.StripPrefix(prefix, fileServer)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setCacheHeaders(w, r.URL.Path)
		stripped.ServeHTTP(w, r)
	})
}

func setCacheHeaders(w http.ResponseWriter, urlPath string) {
	// If the file name carries a hash (a common webpack pattern, e.g.
	// `.[hash].js`), we can safely cache for 7 days. Otherwise a short cache.
	if isLikelyImmutable(urlPath) {
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	_ = time.Now
}

func isLikelyImmutable(p string) bool {
	// Rough heuristic: webpack names usually look like `something.abcd1234.js`.
	// Check that there's a chunk of 8+ hex chars between two dots.
	parts := strings.Split(strings.TrimPrefix(p, "/"), ".")
	if len(parts) < 3 {
		return false
	}
	mid := parts[len(parts)-2]
	if len(mid) < 8 {
		return false
	}
	for _, c := range mid {
		isDigit := c >= '0' && c <= '9'
		isHexLower := c >= 'a' && c <= 'f'
		if !isDigit && !isHexLower {
			return false
		}
	}
	return true
}
