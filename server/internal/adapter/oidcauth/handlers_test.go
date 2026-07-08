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
	idp := oidctest.New(t)
	cfg := &config.CLI{
		OIDCIssuerURL:    idp.IssuerURL(),
		OIDCClientID:     idp.ClientID,
		OIDCClientSecret: "s3cret",
		OIDCCookieSecret: testSecret,
		OIDCSessionTTL:   time.Hour,
		OIDCScopes:       []string{"openid", "profile", "email"},
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

func TestLogout_ClearsSessionCookie(t *testing.T) {
	_, h := testHandlers(t)
	rec := httptest.NewRecorder()
	h.Logout(rec, httptest.NewRequest(http.MethodGet, "/oidc/logout", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
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
