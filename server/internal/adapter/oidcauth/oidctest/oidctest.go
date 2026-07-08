// Package oidctest provides a fake OIDC identity provider and login-flow
// helpers for tests. Test-only, mirrors the role portfakes plays for ports.
//
// The fake IdP serves the discovery document, a JWKS endpoint and a token
// endpoint that issues RS256-signed ID tokens. Because tests never visit the
// authorize endpoint, the `code` passed to the token endpoint doubles as the
// nonce to embed in the issued ID token — SessionCookie extracts the real
// nonce from the login redirect and plays it back as the code.
package oidctest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/sendgo/sendgo/server/internal/adapter/oidcauth"
)

// FakeIDP is an in-process OIDC provider for tests.
type FakeIDP struct {
	Server *httptest.Server
	Key    *rsa.PrivateKey

	// Claims stamped into every issued ID token.
	ClientID string
	Sub      string
	Email    string
	Name     string

	// NoEndSession removes end_session_endpoint from discovery (read at
	// request time — flip it before oidcauth.New runs discovery).
	NoEndSession bool

	// ExtraClaims are merged into every issued ID token (win on conflicts,
	// read at token-request time).
	ExtraClaims map[string]any
}

// New starts a fake IdP. The httptest server URL is the issuer.
func New(t *testing.T) *FakeIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	f := &FakeIDP{
		Key:      key,
		ClientID: "sendgo-test",
		Sub:      "user-123",
		Email:    "user@example.com",
		Name:     "Test User",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		iss := f.Server.URL
		doc := map[string]any{
			"issuer":                                iss,
			"authorization_endpoint":                iss + "/auth",
			"token_endpoint":                        iss + "/token",
			"jwks_uri":                              iss + "/keys",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		}
		if !f.NoEndSession {
			doc["end_session_endpoint"] = iss + "/end-session"
		}
		writeJSON(w, doc)
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		pub := &f.Key.PublicKey
		writeJSON(w, map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": "testkey",
				"use": "sig",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			}},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		// The code doubles as the nonce (see package doc).
		extra := map[string]any{"nonce": r.Form.Get("code")}
		for k, v := range f.ExtraClaims {
			extra[k] = v
		}
		idToken := f.SignIDToken(extra)
		writeJSON(w, map[string]any{
			"access_token": "fake-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idToken,
		})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Server.Close)
	return f
}

// IssuerURL returns the issuer to put into --oidc-issuer-url.
func (f *FakeIDP) IssuerURL() string { return f.Server.URL }

// SignIDToken builds an RS256 JWT with the fake IdP's standard claims merged
// with extra (extra wins on conflicts).
func (f *FakeIDP) SignIDToken(extra map[string]any) string {
	now := time.Now()
	claims := map[string]any{
		"iss":   f.Server.URL,
		"aud":   f.ClientID,
		"sub":   f.Sub,
		"email": f.Email,
		"name":  f.Name,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}
	header := map[string]any{"alg": "RS256", "kid": "testkey"}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	digest := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.Key, crypto.SHA256, digest[:])
	if err != nil {
		panic(fmt.Sprintf("oidctest: sign: %v", err))
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// LoginRedirect performs GET /oidc/login through h and returns the parsed
// authorize URL plus the flow cookie set on the response.
func LoginRedirect(t *testing.T, h *oidcauth.Handlers) (*url.URL, []*http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.Login(rec, httptest.NewRequest(http.MethodGet, "/oidc/login", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("login: got status %d, want 302", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("login: bad Location: %v", err)
	}
	return loc, rec.Result().Cookies()
}

// SessionCookie runs the whole login flow in-memory (login → fake IdP token
// exchange → callback) and returns the resulting session cookie.
func SessionCookie(t *testing.T, h *oidcauth.Handlers) *http.Cookie {
	t.Helper()
	authURL, cookies := LoginRedirect(t, h)
	state := authURL.Query().Get("state")
	nonce := authURL.Query().Get("nonce")
	if state == "" || nonce == "" {
		t.Fatalf("login redirect misses state/nonce: %s", authURL)
	}

	// The nonce rides as the code — the fake token endpoint echoes it into
	// the ID token's nonce claim.
	cb := httptest.NewRequest(http.MethodGet,
		"/oidc/callback?code="+url.QueryEscape(nonce)+"&state="+url.QueryEscape(state), nil)
	for _, c := range cookies {
		cb.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.Callback(rec, cb)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("callback: got status %d, want 303 (body: %s)", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "sendgo_session" && c.Value != "" {
			return c
		}
	}
	t.Fatalf("callback set no session cookie")
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
