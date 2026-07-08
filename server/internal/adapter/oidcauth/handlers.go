package oidcauth

import (
	cryptorand "crypto/rand"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/sendgo/sendgo/server/internal/adapter/observability"
	"github.com/sendgo/sendgo/server/internal/crypto"
)

// Handlers serves GET /oidc/login, /oidc/callback and /oidc/logout.
//
// ResolveBaseURL converts the current request to the public-facing base URL
// (scheme://host) — injected from the http adapter to avoid an import cycle,
// same pattern as UploadWS.ResolveBaseURL.
type Handlers struct {
	Svc            *Service
	ResolveBaseURL func(*http.Request) string
}

// Login starts the authorization code flow: generates state/nonce/PKCE
// verifier, persists them in a short-lived signed cookie and redirects to the
// provider. An already-authenticated visitor is bounced straight back to /
// (loop protection).
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	lg := observability.FromContext(r.Context())
	if h.Svc.SessionFromRequest(r) != nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	state, err := randomToken()
	if err != nil {
		lg.Error("oidc login: state rand", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	nonce, err := randomToken()
	if err != nil {
		lg.Error("oidc login: nonce rand", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	verifier := oauth2.GenerateVerifier()

	now := time.Now()
	flowVal, err := h.Svc.codec.encode(flowState{
		State:    state,
		Nonce:    nonce,
		Verifier: verifier,
		Exp:      now.Add(flowTTL).Unix(),
	})
	if err != nil {
		lg.Error("oidc login: flow cookie encode", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	baseURL := h.ResolveBaseURL(r)
	secure := isHTTPS(baseURL)
	// Path=/oidc: the cookie only travels back on /oidc/callback.
	setCookie(w, flowCookie, flowVal, "/oidc", flowTTL, secure)

	opts := []oauth2.AuthCodeOption{
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	}
	// One-shot marker from a local-only logout: force the account chooser so
	// the provider's live SSO session can't silently log the user back in.
	if _, err := r.Cookie(loggedOutCookie); err == nil {
		clearCookie(w, loggedOutCookie, "/oidc", secure)
		opts = append(opts, oauth2.SetAuthURLParam("prompt", "select_account"))
	}

	conf := h.Svc.oauthConfig(h.Svc.redirectURL(baseURL))
	http.Redirect(w, r, conf.AuthCodeURL(state, opts...), http.StatusFound)
}

// Callback finishes the flow: verifies state against the flow cookie,
// exchanges the code (with the PKCE verifier), validates the ID token and its
// nonce claim, then issues the session cookie.
func (h *Handlers) Callback(w http.ResponseWriter, r *http.Request) {
	lg := observability.FromContext(r.Context())
	baseURL := h.ResolveBaseURL(r)
	secure := isHTTPS(baseURL)

	// The flow cookie is single-use: drop it no matter how the rest goes.
	clearCookie(w, flowCookie, "/oidc", secure)

	var flow flowState
	ck, err := r.Cookie(flowCookie)
	if err != nil || h.Svc.codec.decode(ck.Value, &flow) != nil || expired(flow.Exp, time.Now()) {
		// Stale bookmark, expired attempt or lost cookie — restart the flow
		// instead of dead-ending the user on an error page.
		lg.Info("oidc callback: missing or invalid flow cookie, restarting login")
		http.Redirect(w, r, "/oidc/login", http.StatusFound)
		return
	}

	q := r.URL.Query()
	if errCode := q.Get("error"); errCode != "" {
		lg.Warn("oidc callback: provider returned error", "error", errCode, "description", q.Get("error_description"))
		http.Error(w, "authentication failed: "+errCode, http.StatusBadGateway)
		return
	}
	if q.Get("state") != flow.State {
		lg.Warn("oidc callback: state mismatch")
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}
	code := q.Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	conf := h.Svc.oauthConfig(h.Svc.redirectURL(baseURL))
	token, err := conf.Exchange(r.Context(), code, oauth2.VerifierOption(flow.Verifier))
	if err != nil {
		lg.Error("oidc callback: code exchange failed", "err", err)
		http.Error(w, "token exchange failed", http.StatusBadGateway)
		return
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		lg.Error("oidc callback: token response has no id_token")
		http.Error(w, "no id_token in token response", http.StatusBadGateway)
		return
	}
	idToken, err := h.Svc.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		lg.Warn("oidc callback: id_token verification failed", "err", err)
		http.Error(w, "invalid id_token", http.StatusBadGateway)
		return
	}
	if idToken.Nonce != flow.Nonce {
		lg.Warn("oidc callback: nonce mismatch")
		http.Error(w, "nonce mismatch", http.StatusBadRequest)
		return
	}

	var claims struct {
		Email             string `json:"email"`
		EmailVerified     *bool  `json:"email_verified"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil {
		lg.Warn("oidc callback: claims decode failed", "err", err)
	}
	email := claims.Email
	if email == "" {
		email = claims.PreferredUsername
	}

	if !h.Svc.authorizeEmail(email, claims.EmailVerified) {
		lg.Warn("oidc callback: identity not in allow-list", "sub", idToken.Subject, "email", email)
		http.Error(w, "access denied: this account is not allowed to upload", http.StatusForbidden)
		return
	}

	now := time.Now()
	sessVal, err := h.Svc.codec.encode(Session{
		Sub:   idToken.Subject,
		Email: email,
		Name:  claims.Name,
		Iat:   now.Unix(),
		Exp:   now.Add(h.Svc.cfg.OIDCSessionTTL).Unix(),
	})
	if err != nil {
		lg.Error("oidc callback: session encode", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if !secure {
		// Deliberate: still works (local dev, LAN), but flag it — behind a
		// misconfigured proxy this silently downgrades cookie security.
		lg.Warn("oidc callback: issuing session cookie over plain http (no Secure flag)")
	}
	setCookie(w, sessionCookie, sessVal, "/", h.Svc.cfg.OIDCSessionTTL, secure)
	lg.Info("oidc: user logged in", "sub", idToken.Subject, "email", email)

	// 303: the callback URL (with code/state in the query) must not stay in
	// the address bar or be re-submittable.
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout clears the local session and performs RP-initiated logout at the
// provider (when it advertises an end_session_endpoint). Redirecting to "/"
// instead would silently log the user right back in: "/" bounces to the
// provider, whose live SSO session re-issues a code without showing a login
// form — sign-out would appear to do nothing.
//
// The stateless cookie retains no id_token, so no id_token_hint is sent;
// client_id accompanies post_logout_redirect_uri instead (RP-initiated
// logout spec permits either).
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	baseURL := h.ResolveBaseURL(r)
	secure := isHTTPS(baseURL)
	clearCookie(w, sessionCookie, "/", secure)

	es := h.Svc.endSessionURL
	if es == "" {
		// No RP-initiated logout (e.g. Google). Leave a marker so the next
		// login is interactive — see loggedOutCookie.
		setCookie(w, loggedOutCookie, "1", "/oidc", loggedOutTTL, secure)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	u, err := url.Parse(es)
	if err != nil {
		observability.FromContext(r.Context()).Warn("oidc logout: bad end_session_endpoint", "url", es, "err", err)
		setCookie(w, loggedOutCookie, "1", "/oidc", loggedOutTTL, secure)
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	q := u.Query()
	q.Set("client_id", h.Svc.cfg.OIDCClientID)
	q.Set("post_logout_redirect_uri", baseURL+"/")
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

// randomToken returns 32 random bytes as unpadded URL-safe base64 — used for
// the OAuth2 state and OIDC nonce.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := cryptorand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return crypto.EncodeB64(b), nil
}

// isHTTPS reports whether the public base URL uses https — drives the Secure
// cookie attribute.
func isHTTPS(baseURL string) bool {
	return strings.HasPrefix(baseURL, "https://")
}
