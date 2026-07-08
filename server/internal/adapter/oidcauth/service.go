package oidcauth

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/sendgo/sendgo/server/internal/config"
)

// minSecretLen is the minimum accepted --oidc-cookie-secret length. 32 bytes
// matches the HMAC-SHA256 key size.
const minSecretLen = 32

// Service wraps the discovered OIDC provider plus the session codec. A nil
// *Service means "auth disabled" — callers must treat it as such.
type Service struct {
	cfg      *config.CLI
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	codec    codec
	log      *slog.Logger

	// endSessionURL is the provider's RP-initiated-logout endpoint from
	// discovery ("" when not advertised). Without it a local-only logout is
	// useless in practice: "/" bounces straight back through the provider,
	// whose live SSO session silently re-issues a code — signing out would
	// appear to do nothing.
	endSessionURL string

	// Optional allow-lists (lowercased). Both empty → any authenticated user.
	allowedEmails  map[string]struct{}
	allowedDomains map[string]struct{}
}

// New builds the Service, or returns (nil, nil) when the OIDC flags are not
// set — the same "empty config → feature off" idiom as meta.New/storage.New.
// Partial configuration is a hard error so a typo can't silently disable auth.
//
// Provider discovery runs at startup with ctx; an unreachable IdP fails the
// boot (fail-fast, consistent with the meta/storage factories).
func New(ctx context.Context, cfg *config.CLI, lg *slog.Logger) (*Service, error) {
	issuer, clientID, secret := cfg.OIDCIssuerURL, cfg.OIDCClientID, cfg.OIDCClientSecret
	if issuer == "" && clientID == "" && secret == "" {
		if len(cfg.OIDCAllowedEmails) > 0 || len(cfg.OIDCAllowedDomains) > 0 {
			// A filter without auth would silently protect nothing.
			return nil, fmt.Errorf("oidc: --oidc-allowed-emails/--oidc-allowed-domains require OIDC to be configured")
		}
		return nil, nil // auth disabled
	}
	if issuer == "" || clientID == "" || secret == "" {
		return nil, fmt.Errorf("oidc: --oidc-issuer-url, --oidc-client-id and --oidc-client-secret must be set together")
	}

	cookieSecret := []byte(cfg.OIDCCookieSecret)
	switch {
	case len(cookieSecret) == 0:
		cookieSecret = make([]byte, minSecretLen)
		if _, err := cryptorand.Read(cookieSecret); err != nil {
			return nil, fmt.Errorf("oidc: cookie secret rand: %w", err)
		}
		lg.Warn("oidc: --oidc-cookie-secret not set, using a random per-start key: " +
			"sessions will not survive a restart and multi-replica setups will not work")
	case len(cookieSecret) < minSecretLen:
		return nil, fmt.Errorf("oidc: --oidc-cookie-secret must be at least %d characters", minSecretLen)
	}

	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc: provider discovery (%s): %w", issuer, err)
	}
	var extra struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := provider.Claims(&extra); err == nil && extra.EndSessionEndpoint == "" {
		lg.Warn("oidc: provider advertises no end_session_endpoint — " +
			"sign-out clears the local session only and the SSO session may log users straight back in")
	}

	return &Service{
		cfg:            cfg,
		provider:       provider,
		verifier:       provider.Verifier(&oidc.Config{ClientID: clientID}),
		codec:          codec{secret: cookieSecret},
		log:            lg.With("component", "oidc"),
		endSessionURL:  extra.EndSessionEndpoint,
		allowedEmails:  normalizeSet(cfg.OIDCAllowedEmails, "@"),
		allowedDomains: normalizeSet(cfg.OIDCAllowedDomains, "@"),
	}, nil
}

// normalizeSet lowercases and trims entries (dropping empties) so matching is
// case-insensitive; stripPrefix tolerates "@example.com"-style domain input.
func normalizeSet(values []string, stripPrefix string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		v = strings.ToLower(strings.TrimSpace(v))
		v = strings.TrimPrefix(v, stripPrefix)
		if v != "" {
			set[v] = struct{}{}
		}
	}
	return set
}

// authorizeEmail applies the optional allow-lists to an authenticated
// identity. verified is the id_token's email_verified claim (nil = the
// provider didn't send one; many, e.g. plain Authentik setups, don't — only
// an explicit false rejects).
func (s *Service) authorizeEmail(email string, verified *bool) bool {
	if len(s.allowedEmails) == 0 && len(s.allowedDomains) == 0 {
		return true
	}
	e := strings.ToLower(strings.TrimSpace(email))
	if e == "" {
		return false // can't match an identity the provider didn't disclose
	}
	if verified != nil && !*verified {
		return false // an unverified address must not pass an allow-list
	}
	if _, ok := s.allowedEmails[e]; ok {
		return true
	}
	if _, domain, found := strings.Cut(e, "@"); found {
		if _, ok := s.allowedDomains[domain]; ok {
			return true
		}
	}
	return false
}

// SessionFromRequest returns the authenticated session carried by the request
// cookie, or nil when the cookie is missing, tampered with, or expired.
func (s *Service) SessionFromRequest(r *http.Request) *Session {
	ck, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	var sess Session
	if err := s.codec.decode(ck.Value, &sess); err != nil {
		return nil
	}
	if sess.Sub == "" || expired(sess.Exp, time.Now()) {
		return nil
	}
	return &sess
}

// oauthConfig returns a per-request oauth2.Config copy: redirect_uri may be
// derived from the incoming request (X-Forwarded-*), so it can't be cached.
func (s *Service) oauthConfig(redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     s.cfg.OIDCClientID,
		ClientSecret: s.cfg.OIDCClientSecret,
		Endpoint:     s.provider.Endpoint(),
		RedirectURL:  redirectURL,
		Scopes:       s.cfg.OIDCScopes,
	}
}

// redirectURL resolves the OAuth2 redirect_uri: the explicit flag wins,
// otherwise it's derived from the request's public base URL.
func (s *Service) redirectURL(baseURL string) string {
	if s.cfg.OIDCRedirectURL != "" {
		return s.cfg.OIDCRedirectURL
	}
	return baseURL + "/oidc/callback"
}
