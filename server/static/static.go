// Package static holds the embedded frontend assets.
//
// The dist/ and locales/ directories are copied here from the repo-root dist/
// and public/locales/ before `go build` (justfile recipe prepare-static). The
// Go compiler then embeds them into the binary via //go:embed all:...
//
// If the files are missing (e.g. `go test ./...` without prepare-static), embed
// trivially creates empty subdirectories and endpoints will return 404.
package static

import (
	"embed"
	"encoding/json"
	"io/fs"
)

//go:embed all:dist all:locales
var rootFS embed.FS

// DistFS returns the contents of the root dist/ directory (without the `dist/`
// path prefix).
func DistFS() fs.FS {
	sub, err := fs.Sub(rootFS, "dist")
	if err != nil {
		// embed FS guarantees the directory exists; this only errors if dist/ was
		// missing at build time. Return an "empty" FS.
		return emptyFS{}
	}
	return sub
}

// LocalesFS returns the contents of public/locales/.
func LocalesFS() fs.FS {
	sub, err := fs.Sub(rootFS, "locales")
	if err != nil {
		return emptyFS{}
	}
	return sub
}

// LocaleNames returns the list of locale names (from subdirectory names in
// locales/). Used to build a language.Matcher.
func LocaleNames() []string {
	entries, err := rootFS.ReadDir("locales")
	if err != nil {
		return []string{"en-US"}
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return []string{"en-US"}
	}
	return names
}

// IndexHTML returns the contents of dist/index.html if it was embedded.
// Optional — webpack does not emit index.html for our frontend (the legacy
// Node server used to render it on the fly via layout.js). Only used if
// someone drops their own index.html into dist/.
func IndexHTML() ([]byte, error) {
	return rootFS.ReadFile("dist/index.html")
}

// Manifest reads dist/manifest.json — a map of "name without hash → name with
// hash", produced by the webpack ManifestPlugin (see webpack.config.js).
// Example contents:
//
//	{"app.js": "app.640dfa13.js", "app.css": "app.96a25455.css", ...}
//
// Consumed by the pages handler to render HTML with correct hashed asset
// names. If the file is missing (e.g. in tests), an empty map is returned.
func Manifest() map[string]string {
	data, err := rootFS.ReadFile("dist/manifest.json")
	if err != nil {
		return map[string]string{}
	}
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]string{}
	}
	return m
}

// emptyFS is a stub used when static assets are absent (e.g. during `go test`).
type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) {
	return nil, fs.ErrNotExist
}
