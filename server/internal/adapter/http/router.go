package http

import (
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/sendgo/sendgo/server/internal/adapter/http/handlers"
	mw "github.com/sendgo/sendgo/server/internal/adapter/http/middleware"
	"github.com/sendgo/sendgo/server/internal/adapter/oidcauth"
	"github.com/sendgo/sendgo/server/internal/config"
	"github.com/sendgo/sendgo/server/internal/port"
	"github.com/sendgo/sendgo/server/internal/usecase"
	"github.com/sendgo/sendgo/server/static"
)

// Deps holds everything needed to build the router. Passed in from main.
type Deps struct {
	Cfg            *config.CLI
	Meta           port.MetaStore
	Blob           port.BlobStorage
	Tokens         port.TokenGenerator
	Clock          port.Clock
	InitiateUpload *usecase.InitiateUpload
	StreamUpload   *usecase.StreamUpload
	CheckExists    *usecase.CheckExists
	GetMetadata    *usecase.GetMetadata
	StreamDownload *usecase.StreamDownload
	DeleteShare    *usecase.DeleteShare
	SetPassword    *usecase.SetPassword
	UpdateParams   *usecase.UpdateParams
	GetInfo        *usecase.GetInfo
	// OIDC is nil unless the --oidc-* flags are configured; nil keeps every
	// route public, exactly as before the feature existed.
	OIDC *oidcauth.Service
}

// NewRouter wires up all routes. The API contract matches
// server/routes/index.js 1:1.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	// Common middleware (order matters: RequestID outermost, Recovery just inside).
	r.Use(mw.RequestID)
	r.Use(mw.Recovery)
	r.Use(mw.AccessLog)
	r.Use(mw.SecurityHeaders)

	// `Copy link` in the upload UI yields URLs like /download/<id>/, with a
	// trailing slash by design (mirrors upstream Send). chi route patterns
	// don't fuzzy-match trailing slashes, so without this the download
	// document would fall through the catch-all SPA fallback and lose the
	// pre-populated downloadMetadata — making the page reload-loop on the
	// frontend. StripSlashes rewrites r.URL.Path before route matching.
	r.Use(chimw.StripSlashes)

	// Public pages / static.
	pages := NewPages(d.Cfg, d.Meta)

	// Optional OIDC auth: gate the upload page and register the auth routes.
	// Only the exact "/" route is gated — the SPA fallback, webpack chunks and
	// /download/* stay public (receivers need them); the security boundary for
	// uploads is the /api/ws check below, the "/" redirect is UX.
	if d.OIDC != nil {
		oh := &oidcauth.Handlers{
			Svc:            d.OIDC,
			ResolveBaseURL: func(req *http.Request) string { return BaseURLFromRequest(d.Cfg, req) },
		}
		r.Get("/oidc/login", oh.Login)
		r.Get("/oidc/callback", oh.Callback)
		r.Get("/oidc/logout", oh.Logout)
		r.Get("/", requireSession(d.OIDC, pages.Index))
		// Lets the SPA shell render the visitor's identity (OIDC_AUTH global).
		pages.SessionInfo = d.OIDC.SessionFromRequest
	} else {
		r.Get("/", pages.Index)
	}
	// `/download/{id}` and `/download/{id}/{key}` are SPA routes, but the
	// frontend expects the server to pre-populate `downloadMetadata` and emit
	// `WWW-Authenticate: send-v1 <nonce>` so the very first HMAC-authenticated
	// API request can succeed without a 401-retry loop (matches upstream
	// Send's server/routes/pages.js:download).
	r.Get("/download/{id}", pages.Download)
	r.Get("/download/{id}/{key}", pages.Download)
	r.Handle("/locales/*", StaticHandler("/locales/", static.LocalesFS()))

	// /config
	cfg := &handlers.Config{Cfg: d.Cfg}
	r.Get("/config", cfg.Handle)

	// /__heartbeat__ etc.
	hh := &handlers.Health{Meta: d.Meta, Blob: d.Blob}
	r.Get("/__heartbeat__", hh.Heartbeat)
	r.Get("/__lbheartbeat__", hh.LBHeartbeat)
	r.Get("/__version__", hh.Version)

	// /api/*
	r.Route("/api", func(r chi.Router) {
		// WS upload — the handler asks ResolveBaseURL for the public URL
		// reported back to the client; we plug it with BaseURLFromRequest.
		ws := &handlers.UploadWS{
			Initiate:       d.InitiateUpload,
			Stream:         d.StreamUpload,
			ResolveBaseURL: func(req *http.Request) string { return BaseURLFromRequest(d.Cfg, req) },
		}
		if d.OIDC != nil {
			// Session cookie + same-origin check. The Origin check matters:
			// the upgrader's CheckOrigin is permissive, and once a cookie
			// authorizes uploads a cross-site page could open a WS here with
			// the victim's cookie attached (CSRF). Without OIDC there is no
			// ambient credential, so the permissive default stays.
			ws.Authorize = func(req *http.Request) bool {
				return wsSameOrigin(req) && d.OIDC.SessionFromRequest(req) != nil
			}
		}
		r.HandleFunc("/ws", ws.Handle)

		ex := &handlers.Exists{UC: d.CheckExists}
		r.Get("/exists/{id}", ex.Handle)

		hmacChain := mw.HMAC(mw.HMACDeps{Meta: d.Meta, Tokens: d.Tokens})

		md := &handlers.Metadata{UC: d.GetMetadata}
		r.With(hmacChain).Get("/metadata/{id}", md.Handle)

		dl := &handlers.Download{UC: d.StreamDownload}
		r.With(hmacChain).Get("/download/{id}", dl.Handle)
		r.With(hmacChain).Get("/download/blob/{id}", dl.Handle)

		ownerChain := mw.Owner(mw.OwnerDeps{Meta: d.Meta})

		del := &handlers.Delete{UC: d.DeleteShare}
		r.With(ownerChain).Post("/delete/{id}", del.Handle)

		pw := &handlers.Password{UC: d.SetPassword}
		r.With(ownerChain).Post("/password/{id}", pw.Handle)

		pr := &handlers.Params{UC: d.UpdateParams}
		r.With(ownerChain).Post("/params/{id}", pr.Handle)

		info := &handlers.Info{UC: d.GetInfo}
		r.With(ownerChain).Post("/info/{id}", info.Handle)
	})

	// Catch-all: serve assets from dist/ when the path matches a real file,
	// otherwise fall back to the SPA shell so client-side routes such as
	// `/download/:id/`, `/error`, `/blank`, `/unsupported/:reason` resolve to
	// the choo app instead of returning 404.
	//
	// Webpack is configured with `publicPath: '/'` so code-split chunks
	// (`/0.5a14d529.js` etc.) live at the root — this catch-all sees them
	// after specific routes (`/`, `/api/*`, `/config`, `/__*`, `/locales/*`).
	r.Handle("/*", spaFallback(static.DistFS(), pages))

	return r
}

// requireSession wraps a page handler: without a valid OIDC session the
// visitor is sent to the login flow instead.
func requireSession(svc *oidcauth.Service, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc.SessionFromRequest(r) == nil {
			http.Redirect(w, r, "/oidc/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// wsSameOrigin verifies that a browser-initiated WS handshake comes from our
// own origin. No Origin header (curl, ffsend, native clients) → allowed: such
// clients carry no ambient cookie credential, so CSRF does not apply.
func wsSameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := r.Host
	if v := r.Header.Get("X-Forwarded-Host"); v != "" {
		if i := strings.Index(v, ","); i >= 0 {
			v = v[:i]
		}
		host = strings.TrimSpace(v)
	}
	return u.Host == host
}

// spaFallback returns a handler that tries to serve r.URL.Path from distFS;
// if no such file exists, it renders the SPA index instead (which lets the
// choo app on the client take over routing).
func spaFallback(distFS fs.FS, pages *Pages) http.Handler {
	fileServer := http.FileServer(http.FS(distFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			pages.Index(w, r)
			return
		}
		// Stat the requested path. fs.Stat handles both files and dirs.
		if info, err := fs.Stat(distFS, p); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Missing file → SPA shell. Frontend's choo router reads location.pathname
		// and dispatches to the right view (e.g. `/download/:id` → download UI).
		pages.Index(w, r)
	})
}

// BaseURLFromRequest resolves the public base URL for a single request.
//
// Priority:
//  1. If --detect-base-url is OFF and --base-url is set explicitly → use it.
//  2. Otherwise: scheme from request (r.TLS / X-Forwarded-Proto, defaults to
//     http) plus r.Host. This is the right default for local development
//     over plain http on 127.0.0.1, and for production behind a TLS-terminating
//     reverse proxy that sets X-Forwarded-Proto.
func BaseURLFromRequest(cfg *config.CLI, r *http.Request) string {
	if !cfg.DetectBaseURL && cfg.BaseURL != "" {
		return cfg.BaseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if v := r.Header.Get("X-Forwarded-Proto"); v != "" {
		// Trust the first value (`proto1, proto2`).
		if i := strings.Index(v, ","); i >= 0 {
			v = v[:i]
		}
		scheme = strings.TrimSpace(v)
	}
	host := r.Host
	if v := r.Header.Get("X-Forwarded-Host"); v != "" {
		if i := strings.Index(v, ","); i >= 0 {
			v = v[:i]
		}
		host = strings.TrimSpace(v)
	}
	return scheme + "://" + host
}
