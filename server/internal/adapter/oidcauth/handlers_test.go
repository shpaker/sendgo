package oidcauth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/sendgo/sendgo/server/internal/adapter/oidcauth"
	"github.com/sendgo/sendgo/server/internal/adapter/oidcauth/oidctest"
	"github.com/sendgo/sendgo/server/internal/config"
)

// testHandlers builds Handlers wired to a fake IdP.
func testHandlers(t *testing.T) (*oidctest.FakeIDP, *oidcauth.Handlers) {
	t.Helper()
	return testHandlersOpts(t, nil)
}

// testHandlersOpts is testHandlers with a config hook.
func testHandlersOpts(t *testing.T, mutate func(*config.CLI)) (*oidctest.FakeIDP, *oidcauth.Handlers) {
	t.Helper()
	idp := oidctest.New(t)
	cfg := &config.CLI{
		OIDCIssuerURL:    idp.IssuerURL(),
		OIDCClientID:     idp.ClientID,
		OIDCClientSecret: "s3cret",
		OIDCCookieSecret: testSecret,
		OIDCSessionTTL:   time.Hour,
		OIDCScopes:       []string{"openid", "profile", "email"},
	}
	if mutate != nil {
		mutate(cfg)
	}
	svc, err := oidcauth.New(context.Background(), cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return idp, &oidcauth.Handlers{
		Svc:            svc,
		ResolveBaseURL: func(*http.Request) string { return "http://sendgo.test" },
	}
}

// doCallback runs login + a well-formed callback and returns the raw
// response — for tests that expect the callback to refuse a session.
func doCallback(t *testing.T, h *oidcauth.Handlers) *httptest.ResponseRecorder {
	t.Helper()
	authURL, cookies := oidctest.LoginRedirect(t, h)
	state := authURL.Query().Get("state")
	nonce := authURL.Query().Get("nonce")
	req := httptest.NewRequest(http.MethodGet,
		"/oidc/callback?code="+url.QueryEscape(nonce)+"&state="+url.QueryEscape(state), nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.Callback(rec, req)
	return rec
}

func TestLogin_RedirectsWithStatePKCENonce(t *testing.T) {
	idp, h := testHandlers(t)
	authURL, cookies := oidctest.LoginRedirect(t, h)

	q := authURL.Query()
	if q.Get("state") == "" {
		t.Error("no state in authorize URL")
	}
	if q.Get("nonce") == "" {
		t.Error("no nonce in authorize URL")
	}
	if q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" {
		t.Errorf("PKCE missing: challenge=%q method=%q", q.Get("code_challenge"), q.Get("code_challenge_method"))
	}
	if got, want := q.Get("client_id"), idp.ClientID; got != want {
		t.Errorf("client_id = %q, want %q", got, want)
	}
	if got, want := q.Get("redirect_uri"), "http://sendgo.test/oidc/callback"; got != want {
		t.Errorf("redirect_uri = %q, want %q", got, want)
	}

	var flow *http.Cookie
	for _, c := range cookies {
		if c.Name == "sendgo_oidc_flow" {
			flow = c
		}
	}
	if flow == nil || flow.Value == "" {
		t.Fatal("no flow cookie set")
	}
	if flow.Path != "/oidc" || !flow.HttpOnly {
		t.Errorf("flow cookie attrs: path=%q httponly=%v", flow.Path, flow.HttpOnly)
	}
}

func TestCallback_HappyPathSetsSessionCookie(t *testing.T) {
	idp, h := testHandlers(t)
	ck := oidctest.SessionCookie(t, h)

	if ck.Path != "/" || !ck.HttpOnly {
		t.Errorf("session cookie attrs: path=%q httponly=%v", ck.Path, ck.HttpOnly)
	}

	// The cookie must round-trip through SessionFromRequest.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(ck)
	sess := h.Svc.SessionFromRequest(r)
	if sess == nil {
		t.Fatal("SessionFromRequest rejected freshly issued cookie")
	}
	if sess.Sub != idp.Sub || sess.Email != idp.Email || sess.Name != idp.Name {
		t.Errorf("session claims = %+v, want sub=%q email=%q name=%q", sess, idp.Sub, idp.Email, idp.Name)
	}
}

func TestCallback_StateMismatch(t *testing.T) {
	_, h := testHandlers(t)
	authURL, cookies := oidctest.LoginRedirect(t, h)
	nonce := authURL.Query().Get("nonce")

	req := httptest.NewRequest(http.MethodGet,
		"/oidc/callback?code="+url.QueryEscape(nonce)+"&state=WRONG", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.Callback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestCallback_MissingFlowCookieRestartsLogin(t *testing.T) {
	_, h := testHandlers(t)
	rec := httptest.NewRecorder()
	h.Callback(rec, httptest.NewRequest(http.MethodGet, "/oidc/callback?code=x&state=y", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/oidc/login" {
		t.Errorf("Location = %q, want /oidc/login", loc)
	}
}

func TestCallback_NonceMismatch(t *testing.T) {
	// Pass a code that differs from the nonce: the fake IdP stamps the code
	// value into the ID token's nonce claim, so verification must fail.
	_, h := testHandlers(t)
	authURL, cookies := oidctest.LoginRedirect(t, h)
	state := authURL.Query().Get("state")

	req := httptest.NewRequest(http.MethodGet,
		"/oidc/callback?code=not-the-nonce&state="+url.QueryEscape(state), nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.Callback(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (nonce mismatch)", rec.Code)
	}
}

func TestCallback_ProviderError(t *testing.T) {
	_, h := testHandlers(t)
	_, cookies := oidctest.LoginRedirect(t, h)

	req := httptest.NewRequest(http.MethodGet,
		"/oidc/callback?error=access_denied&error_description=nope", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.Callback(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestLogin_AuthenticatedUserBouncesHome(t *testing.T) {
	_, h := testHandlers(t)
	ck := oidctest.SessionCookie(t, h)

	req := httptest.NewRequest(http.MethodGet, "/oidc/login", nil)
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	h.Login(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q, want /", loc)
	}
}

func TestLogout_ClearsCookieAndEndsProviderSession(t *testing.T) {
	idp, h := testHandlers(t)
	rec := httptest.NewRecorder()
	h.Logout(rec, httptest.NewRequest(http.MethodGet, "/oidc/logout", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("bad Location: %v", err)
	}
	// RP-initiated logout: must leave for the provider's end-session endpoint,
	// otherwise the live SSO session silently logs the user right back in.
	if got, want := loc.Scheme+"://"+loc.Host+loc.Path, idp.IssuerURL()+"/end-session"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if loc.Query().Get("client_id") != idp.ClientID {
		t.Errorf("client_id = %q, want %q", loc.Query().Get("client_id"), idp.ClientID)
	}
	if got, want := loc.Query().Get("post_logout_redirect_uri"), "http://sendgo.test/"; got != want {
		t.Errorf("post_logout_redirect_uri = %q, want %q", got, want)
	}
	var cleared bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == "sendgo_session" && c.MaxAge < 0 && c.Value == "" {
			cleared = true
		}
	}
	if !cleared {
		t.Error("session cookie not cleared")
	}
}

// testHandlersNoEndSession builds Handlers against an IdP that advertises no
// end_session_endpoint (like Google).
func testHandlersNoEndSession(t *testing.T) *oidcauth.Handlers {
	t.Helper()
	idp := oidctest.New(t)
	idp.NoEndSession = true
	cfg := &config.CLI{
		OIDCIssuerURL:    idp.IssuerURL(),
		OIDCClientID:     idp.ClientID,
		OIDCClientSecret: "s3cret",
		OIDCCookieSecret: testSecret,
		OIDCSessionTTL:   time.Hour,
	}
	svc, err := oidcauth.New(context.Background(), cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &oidcauth.Handlers{Svc: svc, ResolveBaseURL: func(*http.Request) string { return "http://sendgo.test" }}
}

func TestLogout_LocalOnlyWithoutEndSessionEndpoint(t *testing.T) {
	h := testHandlersNoEndSession(t)

	rec := httptest.NewRecorder()
	h.Logout(rec, httptest.NewRequest(http.MethodGet, "/oidc/logout", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Errorf("Location = %q, want / (local-only fallback)", loc)
	}
	// The one-shot logged-out marker must be set so the next login forces an
	// account chooser instead of a silent SSO re-login.
	var marker *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "sendgo_logged_out" && c.Value != "" && c.MaxAge > 0 {
			marker = c
		}
	}
	if marker == nil {
		t.Fatal("logged-out marker cookie not set")
	}
}

func TestLogin_ForcesAccountChooserAfterLocalLogout(t *testing.T) {
	h := testHandlersNoEndSession(t)

	req := httptest.NewRequest(http.MethodGet, "/oidc/login", nil)
	req.AddCookie(&http.Cookie{Name: "sendgo_logged_out", Value: "1"})
	rec := httptest.NewRecorder()
	h.Login(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("bad Location: %v", err)
	}
	if got := loc.Query().Get("prompt"); got != "select_account" {
		t.Errorf("prompt = %q, want select_account", got)
	}
	// Marker is one-shot: it must be cleared alongside.
	var cleared bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == "sendgo_logged_out" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("logged-out marker not cleared by login")
	}
}

func TestCallback_RejectsEmailNotInAllowList(t *testing.T) {
	// Fake IdP issues user@example.com; only boss@corp.com is allowed.
	_, h := testHandlersOpts(t, func(cfg *config.CLI) {
		cfg.OIDCAllowedEmails = []string{"boss@corp.com"}
	})
	rec := doCallback(t, h)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "sendgo_session" && c.Value != "" {
			t.Error("session cookie issued despite allow-list rejection")
		}
	}
}

func TestCallback_AllowsListedDomain(t *testing.T) {
	idp, h := testHandlersOpts(t, func(cfg *config.CLI) {
		cfg.OIDCAllowedDomains = []string{"@Example.COM"} // normalization: @-prefix + case
	})
	ck := oidctest.SessionCookie(t, h) // fatals unless callback returns 303 + cookie
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(ck)
	if sess := h.Svc.SessionFromRequest(r); sess == nil || sess.Email != idp.Email {
		t.Fatalf("expected valid session for %s, got %+v", idp.Email, sess)
	}
}

func TestCallback_AllowsListedEmailCaseInsensitive(t *testing.T) {
	_, h := testHandlersOpts(t, func(cfg *config.CLI) {
		cfg.OIDCAllowedEmails = []string{"USER@example.com"}
	})
	oidctest.SessionCookie(t, h)
}

func TestCallback_RejectsUnverifiedEmailWhenFiltering(t *testing.T) {
	idp, h := testHandlersOpts(t, func(cfg *config.CLI) {
		cfg.OIDCAllowedDomains = []string{"example.com"}
	})
	idp.ExtraClaims = map[string]any{"email_verified": false}
	rec := doCallback(t, h)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (unverified email must not pass an allow-list)", rec.Code)
	}
}

func TestNew_FilterWithoutOIDCFails(t *testing.T) {
	cfg := &config.CLI{OIDCAllowedDomains: []string{"example.com"}}
	if _, err := oidcauth.New(context.Background(), cfg, testLogger()); err == nil {
		t.Error("allow-list without OIDC core config accepted")
	}
}

func TestLogin_NoPromptWithoutLogoutMarker(t *testing.T) {
	_, h := testHandlers(t)
	authURL, _ := oidctest.LoginRedirect(t, h)
	if p := authURL.Query().Get("prompt"); p != "" {
		t.Errorf("prompt = %q, want empty on a regular login", p)
	}
}
