package http

import (
	"bytes"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/sendgo/sendgo/server/internal/adapter/observability"
	"github.com/sendgo/sendgo/server/internal/config"
	"github.com/sendgo/sendgo/server/internal/crypto"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/i18n"
	"github.com/sendgo/sendgo/server/internal/port"
	"github.com/sendgo/sendgo/server/internal/usecase"
	"github.com/sendgo/sendgo/server/static"
)

// Pages renders the HTML shell for SPA routes. It is a minimal SSR pipeline:
// it substitutes <html lang>, hashed asset references from dist/manifest.json,
// branding tokens, and an inline <script> that declares the globals the
// frontend's `app/main.js` expects (LIMITS / WEB_UI / DEFAULTS / PREFS /
// downloadMetadata). The body is empty: choo (app.js) takes over and renders
// the route on the client.
//
// This is a simplified port of upstream Send's
// server/layout.js + server/pages/index.js + server/initScript.js. Full SSR
// through choo is not reproduced — the client rewrites the DOM anyway.
type Pages struct {
	negotiator *i18n.Negotiator
	tmpl       *template.Template
	cfg        *config.CLI
	meta       port.MetaStore
	manifest   map[string]string

	// Pre-serialized constants — they never change after startup.
	limitsJSON   template.JS
	webUIJSON    template.JS
	defaultsJSON template.JS
}

// NewPages constructs the renderer: it initializes the locale negotiator,
// parses the embedded HTML template, and loads the webpack manifest.
func NewPages(cfg *config.CLI, meta port.MetaStore) *Pages {
	tmpl := template.Must(template.New("index").Parse(indexTemplate))

	constants := usecase.BuildClientConstants(cfg)
	limitsB, _ := json.Marshal(constants.Limits)
	webUIB, _ := json.Marshal(constants.WebUI)
	defaultsB, _ := json.Marshal(constants.Defaults)

	return &Pages{
		negotiator: i18n.NewNegotiator(static.LocaleNames()),
		tmpl:       tmpl,
		cfg:        cfg,
		meta:       meta,
		manifest:   static.Manifest(),
		// template.JS bypasses html/template's auto-escape on purpose: we're
		// inlining structurally-validated JSON produced by json.Marshal, not
		// user input. gosec G203 false-positive.
		limitsJSON:   template.JS(limitsB),   //nolint:gosec
		webUIJSON:    template.JS(webUIB),    //nolint:gosec
		defaultsJSON: template.JS(defaultsB), //nolint:gosec
	}
}

// pageData is the data passed into the HTML template.
type pageData struct {
	Lang              string
	Title             string
	Description       string
	BaseURL           string
	AppJS             string // hashed name from manifest, e.g. "app.640dfa13.js"
	AppCSS            string
	WordmarkURL       string
	FaviconURL        string
	Favicon16URL      string
	Favicon32URL      string
	AppleTouchIconURL string
	SafariPinnedTab   string
	OGImageURL        string
	TwitterImageURL   string
	ColorPrimary      string
	ColorAccent       string
	CustomCSS         string // raw URL if any
	CustomCSSInline   template.CSS

	// Per-request CSP nonce for the inline <script>.
	CSPNonce string

	// Globals injected into the inline script (consumed by app/main.js).
	LimitsJSON           template.JS
	WebUIJSON            template.JS
	DefaultsJSON         template.JS
	DownloadMetadataJSON template.JS
}

// Index is the handler for `GET /`.
func (p *Pages) Index(w http.ResponseWriter, r *http.Request) {
	p.render(w, r, http.StatusOK, template.JS("{}"))
}

// Download is the handler for `GET /download/{id}` and
// `GET /download/{id}/{key}`. It mirrors upstream Send's
// server/routes/pages.js:download — it loads the share, pre-populates
// `downloadMetadata = {nonce, pwd}` for the frontend, and sets the
// WWW-Authenticate header with the current nonce so the client can do its
// first HMAC-authenticated request without an extra 401 round-trip.
//
// If the share is unknown, the page renders with downloadMetadata.status =
// 404 (still a 200 page so the SPA can show the "not found" UI).
func (p *Pages) Download(w http.ResponseWriter, r *http.Request) {
	lg := observability.FromContext(r.Context())
	idStr := chi.URLParam(r, "id")
	if !domain.ValidShareID(idStr) {
		p.renderDownloadStatus(w, r, http.StatusNotFound)
		return
	}
	share, err := p.meta.Get(r.Context(), domain.ShareID(idStr))
	if errors.Is(err, domain.ErrNotFound) {
		p.renderDownloadStatus(w, r, http.StatusNotFound)
		return
	}
	if err != nil {
		lg.Error("pages: meta.Get failed for download page", "err", err, "share_id", idStr)
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}

	nonceB64 := crypto.EncodeB64(share.Nonce)
	w.Header().Set("WWW-Authenticate", "send-v1 "+nonceB64)

	// {nonce, pwd} — same shape upstream Node sends. json.Marshal of a struct
	// is safe to inline (gosec G203 doesn't apply: no user input).
	dl := struct {
		Nonce string `json:"nonce"`
		Pwd   bool   `json:"pwd"`
	}{Nonce: nonceB64, Pwd: share.Password}
	dlBytes, _ := json.Marshal(dl)
	p.render(w, r, http.StatusOK, template.JS(dlBytes)) //nolint:gosec
}

// renderDownloadStatus renders the SPA with downloadMetadata.status set to
// the given HTTP status — used when the share id is unknown / invalid so the
// frontend can show the "not found" view.
func (p *Pages) renderDownloadStatus(w http.ResponseWriter, r *http.Request, status int) {
	payload, _ := json.Marshal(map[string]int{"status": status})
	p.render(w, r, status, template.JS(payload)) //nolint:gosec
}

// render is the common HTML rendering path used by Index and Download.
func (p *Pages) render(w http.ResponseWriter, r *http.Request, status int, downloadMetadataJSON template.JS) {
	lang := p.negotiator.Negotiate(r.Header.Get("Accept-Language"))
	if p.cfg.CustomLocale != "" {
		lang = p.cfg.CustomLocale
	}

	// All assets live at the root (webpack is configured with `publicPath: '/'`)
	// and the catch-all route in router.go serves them from the embed.FS dist/.
	asset := func(name, fallback string) string {
		if v, ok := p.manifest[name]; ok && v != "" {
			return "/" + v
		}
		return fallback
	}

	nonce, err := makeCSPNonce()
	if err != nil {
		observability.FromContext(r.Context()).Error("pages: nonce gen failed", "err", err)
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}

	data := pageData{
		Lang:              lang,
		Title:             firstNonEmpty(p.cfg.CustomTitle, "Send"),
		Description:       firstNonEmpty(p.cfg.CustomDescription, "Encrypted file sharing"),
		BaseURL:           p.cfg.BaseURL,
		AppJS:             asset("app.js", ""),
		AppCSS:            asset("app.css", ""),
		WordmarkURL:       firstNonEmpty(p.cfg.UICustomAssetsWordmark, asset("wordmark.svg", "")),
		FaviconURL:        firstNonEmpty(p.cfg.UICustomAssetsIcon, asset("favicon.ico", "")),
		Favicon16URL:      firstNonEmpty(p.cfg.UICustomAssetsFavicon16, asset("favicon-16x16.png", "")),
		Favicon32URL:      firstNonEmpty(p.cfg.UICustomAssetsFavicon32, asset("favicon-32x32.png", "")),
		AppleTouchIconURL: firstNonEmpty(p.cfg.UICustomAssetsAppleTouchIcon, asset("apple-touch-icon.png", "")),
		SafariPinnedTab:   firstNonEmpty(p.cfg.UICustomAssetsSafariPinnedTab, asset("safari-pinned-tab.svg", "")),
		OGImageURL:        firstNonEmpty(p.cfg.UICustomAssetsFacebook, asset("send-fb.jpg", "")),
		TwitterImageURL:   firstNonEmpty(p.cfg.UICustomAssetsTwitter, asset("send-twitter.jpg", "")),
		ColorPrimary:      p.cfg.UIColorPrimary,
		ColorAccent:       p.cfg.UIColorAccent,
		CustomCSS:         p.cfg.UICustomCSS,
		// Admin-configured CSS gets inlined verbatim by design (mirrors upstream
		// Send's UI_CUSTOM_CSS). gosec G203 is irrelevant — it's not user input.
		CustomCSSInline: template.CSS(p.cfg.UICustomCSS), //nolint:gosec

		CSPNonce:             nonce,
		LimitsJSON:           p.limitsJSON,
		WebUIJSON:            p.webUIJSON,
		DefaultsJSON:         p.defaultsJSON,
		DownloadMetadataJSON: downloadMetadataJSON,
	}

	var buf bytes.Buffer
	if err := p.tmpl.Execute(&buf, data); err != nil {
		lg := observability.FromContext(r.Context())
		lg.Error("pages: render failed", "err", err)
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}

	// CSP with a per-request nonce permits only our own inline <script>.
	// Overrides the header set by the security middleware.
	w.Header().Set("Content-Security-Policy", fmt.Sprintf(
		"default-src 'self'; "+
			"script-src 'self' 'wasm-unsafe-eval' 'nonce-%[1]s'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data: blob:; "+
			"font-src 'self' data:; "+
			"connect-src 'self' ws: wss:; "+
			"worker-src 'self' blob:; "+
			"frame-ancestors 'none'",
		nonce,
	))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// makeCSPNonce returns 16 random bytes in base64 — fresh value per request.
func makeCSPNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := cryptorand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// indexTemplate is a port of server/layout.js + server/initScript.js. body is
// intentionally empty: choo (app.js) initializes the app after load and
// rewrites the DOM. The inline <script> declares the globals expected by
// app/main.js (LIMITS / WEB_UI / DEFAULTS / PREFS / downloadMetadata).
//
//nolint:lll
const indexTemplate = `<!DOCTYPE html>
<html lang="{{.Lang}}">
<head>
  <title>{{.Title}}</title>
  <base href="/" />
  <meta name="robots" content="noindex,noarchive" />
  <meta name="google" content="nositelinkssearchbox" />
  <meta http-equiv="X-UA-Compatible" content="IE=edge" />
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <meta name="description" content="{{.Description}}" />
  <meta property="og:title" content="{{.Title}}" />
  <meta property="og:description" content="{{.Description}}" />
  <meta property="og:image" content="{{.OGImageURL}}" />
  <meta property="og:url" content="{{.BaseURL}}" />
  <meta name="twitter:title" content="{{.Title}}" />
  <meta name="twitter:description" content="{{.Description}}" />
  <meta name="twitter:card" content="summary" />
  <meta name="twitter:image" content="{{.TwitterImageURL}}" />
  <meta name="theme-color" content="#220033" />
  <meta name="msapplication-TileColor" content="#220033" />
  <link rel="manifest" href="/manifest.json" />
  <link rel="stylesheet" type="text/css" href="/inter.css" />
  <style>
    :root {
      --color-primary: {{.ColorPrimary}};
      --color-primary-accent: {{.ColorAccent}};
    }
    {{.CustomCSSInline}}
  </style>
  {{if .AppCSS}}<link rel="stylesheet" type="text/css" href="{{.AppCSS}}" />{{end}}
  {{if .CustomCSS}}<link rel="stylesheet" type="text/css" href="{{.CustomCSS}}" />{{end}}
  {{if .AppleTouchIconURL}}<link rel="apple-touch-icon" sizes="180x180" href="{{.AppleTouchIconURL}}" />{{end}}
  {{if .Favicon32URL}}<link rel="icon" type="image/png" sizes="32x32" href="{{.Favicon32URL}}" />{{end}}
  {{if .Favicon16URL}}<link rel="icon" type="image/png" sizes="16x16" href="{{.Favicon16URL}}" />{{end}}
  {{if .SafariPinnedTab}}<link rel="mask-icon" href="{{.SafariPinnedTab}}" color="#838383" />{{end}}
  {{if .FaviconURL}}<link rel="shortcut icon" href="{{.FaviconURL}}" />{{end}}
  <script nonce="{{.CSPNonce}}">
    var LIMITS = {{.LimitsJSON}};
    var WEB_UI = {{.WebUIJSON}};
    var DEFAULTS = {{.DefaultsJSON}};
    var PREFS = {};
    var downloadMetadata = {{.DownloadMetadataJSON}};
  </script>
  {{if .AppJS}}<script defer src="{{.AppJS}}"></script>{{end}}
</head>
<body>
  <noscript>
    <div class="noscript">
      <h2>JavaScript is required.</h2>
      <p>Please enable JavaScript to use sendgo.</p>
    </div>
  </noscript>
</body>
</html>
`
