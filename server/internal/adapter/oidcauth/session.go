// Package oidcauth implements optional OIDC authentication for uploads.
//
// When the --oidc-* flags are unset the package is inert: New returns nil and
// the router registers nothing, so the server behaves exactly as without it.
// When configured, unauthenticated visitors of the upload page are redirected
// to the identity provider (authorization code flow + PKCE) and /api/ws
// rejects uploads without a valid session cookie. Downloads stay public.
//
// Sessions are stateless: an HMAC-signed cookie carries the identity claims
// and expiry, so no server-side session store is needed and any replica with
// the same cookie secret can validate it.
package oidcauth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sendgo/sendgo/server/internal/crypto"
)

const (
	// sessionCookie holds the signed Session after a successful login.
	sessionCookie = "sendgo_session"
	// flowCookie holds the signed flowState while the OAuth2 dance with the
	// provider is in flight (login → callback). Scoped to /oidc only.
	flowCookie = "sendgo_oidc_flow"
	// flowTTL bounds how long a login attempt may take.
	flowTTL = 10 * time.Minute
	// loggedOutCookie is a one-shot marker set by a local-only logout (the
	// provider advertises no end_session_endpoint, e.g. Google). The next
	// login adds prompt=select_account so the still-alive SSO session shows
	// an account chooser instead of silently logging the user back in.
	// A plain unsigned flag: forging it only makes the chooser appear.
	loggedOutCookie = "sendgo_logged_out"
	loggedOutTTL    = 5 * time.Minute
	// expLeeway absorbs clock skew between replicas when checking Exp.
	expLeeway = 30 * time.Second
)

// Session is the authenticated-user payload stored in the session cookie.
type Session struct {
	Sub   string `json:"sub"`
	Email string `json:"email,omitempty"`
	Name  string `json:"name,omitempty"`
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"` // unix seconds
}

// flowState is the transient payload stored in the flow cookie between the
// redirect to the provider and the callback.
type flowState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"` // PKCE code_verifier
	Exp      int64  `json:"exp"`
}

// codec signs and verifies cookie payloads. Format:
//
//	b64url(json) + "." + b64url(HMAC-SHA256(secret, b64url(json)))
//
// Signature only — the payload is readable by the client (it contains nothing
// secret), but not forgeable.
type codec struct{ secret []byte }

func (c codec) encode(payload any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("oidc session encode: %w", err)
	}
	body := crypto.EncodeB64(raw)
	sig := crypto.ComputeHMAC(c.secret, []byte(body))
	return body + "." + crypto.EncodeB64(sig), nil
}

// decode verifies the signature and unmarshals the payload into `into`.
// Expiry is NOT checked here — callers compare the decoded Exp themselves.
func (c codec) decode(value string, into any) error {
	body, sigB64, ok := strings.Cut(value, ".")
	if !ok {
		return fmt.Errorf("oidc session decode: malformed value")
	}
	sig, err := crypto.DecodeB64(sigB64)
	if err != nil {
		return fmt.Errorf("oidc session decode: bad signature encoding: %w", err)
	}
	if !crypto.VerifyHMAC(c.secret, []byte(body), sig) {
		return fmt.Errorf("oidc session decode: signature mismatch")
	}
	raw, err := crypto.DecodeB64(body)
	if err != nil {
		return fmt.Errorf("oidc session decode: bad payload encoding: %w", err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("oidc session decode: bad payload json: %w", err)
	}
	return nil
}

// expired reports whether a unix-seconds deadline has passed, with leeway for
// clock skew across replicas.
func expired(expUnix int64, now time.Time) bool {
	return now.After(time.Unix(expUnix, 0).Add(expLeeway))
}

// setCookie writes a signed cookie. `secure` must reflect the public scheme
// (https behind a TLS-terminating proxy → true), not the local listener.
//
// SameSite=Lax is deliberate: the IdP→callback hop is a cross-site top-level
// GET and Strict would withhold the flow cookie, breaking state verification.
func setCookie(w http.ResponseWriter, name, value, path string, maxAge time.Duration, secure bool) {
	// gosec G124 wants a statically-true Secure flag; ours is derived from the
	// public scheme because plain-HTTP deployments (local dev, LAN) are
	// supported deliberately. HttpOnly and SameSite are always set.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearCookie expires a cookie immediately.
func clearCookie(w http.ResponseWriter, name, path string, secure bool) {
	// Same G124 story as setCookie: Secure is conditional by design.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec
		Name:     name,
		Value:    "",
		Path:     path,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
