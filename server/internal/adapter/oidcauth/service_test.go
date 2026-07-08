package oidcauth_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sendgo/sendgo/server/internal/adapter/oidcauth"
	"github.com/sendgo/sendgo/server/internal/adapter/oidcauth/oidctest"
	"github.com/sendgo/sendgo/server/internal/config"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestNew_DisabledWhenUnconfigured(t *testing.T) {
	svc, err := oidcauth.New(context.Background(), &config.CLI{}, testLogger())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc != nil {
		t.Error("expected nil service with empty config")
	}
}

func TestNew_PartialConfigFails(t *testing.T) {
	cases := []config.CLI{
		{OIDCIssuerURL: "https://idp.example.com"},
		{OIDCClientID: "id"},
		{OIDCClientSecret: "secret"},
		{OIDCIssuerURL: "https://idp.example.com", OIDCClientID: "id"},
	}
	for i, cfg := range cases {
		if _, err := oidcauth.New(context.Background(), &cfg, testLogger()); err == nil {
			t.Errorf("case %d: partial config accepted", i)
		} else if !strings.Contains(err.Error(), "must be set together") {
			t.Errorf("case %d: unexpected error: %v", i, err)
		}
	}
}

func TestNew_ShortCookieSecretFails(t *testing.T) {
	idp := oidctest.New(t)
	cfg := &config.CLI{
		OIDCIssuerURL:    idp.IssuerURL(),
		OIDCClientID:     idp.ClientID,
		OIDCClientSecret: "s3cret",
		OIDCCookieSecret: "too-short",
	}
	if _, err := oidcauth.New(context.Background(), cfg, testLogger()); err == nil {
		t.Error("short cookie secret accepted")
	}
}

func TestNew_DiscoverySucceeds(t *testing.T) {
	idp := oidctest.New(t)
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
	if svc == nil {
		t.Fatal("expected enabled service")
	}
}

func TestNew_DiscoveryFailureFails(t *testing.T) {
	// A server that 404s discovery.
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	cfg := &config.CLI{
		OIDCIssuerURL:    srv.URL,
		OIDCClientID:     "id",
		OIDCClientSecret: "s3cret",
		OIDCCookieSecret: testSecret,
	}
	if _, err := oidcauth.New(context.Background(), cfg, testLogger()); err == nil {
		t.Error("discovery failure accepted")
	}
}

func TestSessionFromRequest_NoCookie(t *testing.T) {
	svc := testService(t)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if svc.SessionFromRequest(r) != nil {
		t.Error("session without cookie")
	}
}

func TestSessionFromRequest_GarbageCookie(t *testing.T) {
	svc := testService(t)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: "sendgo_session", Value: "garbage.value"})
	if svc.SessionFromRequest(r) != nil {
		t.Error("session from garbage cookie")
	}
}

// testService builds an enabled Service backed by a fake IdP.
func testService(t *testing.T) *oidcauth.Service {
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
	return svc
}
