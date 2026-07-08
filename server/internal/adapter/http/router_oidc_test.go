package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sendgo/sendgo/server/internal/adapter/oidcauth"
	"github.com/sendgo/sendgo/server/internal/adapter/oidcauth/oidctest"
	"github.com/sendgo/sendgo/server/internal/config"
	"github.com/sendgo/sendgo/server/internal/crypto"
	"github.com/sendgo/sendgo/server/internal/domain"
	"github.com/sendgo/sendgo/server/internal/port/portfakes"
	"github.com/sendgo/sendgo/server/internal/usecase"
)

const testCookieSecret = "0123456789abcdef0123456789abcdef"

// testDeps builds a minimal working Deps: fake stores, real token generator,
// only the upload use cases wired (the rest are not exercised here).
func testDeps(cfg *config.CLI) Deps {
	meta := portfakes.NewFakeMetaStore()
	blob := portfakes.NewFakeBlobStorage()
	tokens := crypto.RandomTokens{}
	clk := portfakes.NewFakeClock(time.Now())
	return Deps{
		Cfg:    cfg,
		Meta:   meta,
		Blob:   blob,
		Tokens: tokens,
		Clock:  clk,
		InitiateUpload: &usecase.InitiateUpload{
			Tokens:   tokens,
			MaxFile:  1 << 30,
			MaxTTL:   604800,
			MaxDL:    100,
			Defaults: usecase.Defaults{ExpireSeconds: 86400, Downloads: 1},
		},
		StreamUpload: &usecase.StreamUpload{
			Blob:    blob,
			Meta:    meta,
			MaxSize: 1 << 30,
			Clock:   clk,
		},
	}
}

// enabledOIDC spins up a fake IdP, builds an enabled Service and returns it
// together with a valid session cookie.
func enabledOIDC(t *testing.T) (*oidcauth.Service, *http.Cookie) {
	t.Helper()
	idp := oidctest.New(t)
	cfg := &config.CLI{
		OIDCIssuerURL:    idp.IssuerURL(),
		OIDCClientID:     idp.ClientID,
		OIDCClientSecret: "s3cret",
		OIDCCookieSecret: testCookieSecret,
		OIDCSessionTTL:   time.Hour,
		OIDCScopes:       []string{"openid", "profile", "email"},
	}
	svc, err := oidcauth.New(context.Background(), cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("oidcauth.New: %v", err)
	}
	h := &oidcauth.Handlers{
		Svc:            svc,
		ResolveBaseURL: func(*http.Request) string { return "http://sendgo.test" },
	}
	return svc, oidctest.SessionCookie(t, h)
}

func TestIndex_RedirectsAnonymousWhenOIDCEnabled(t *testing.T) {
	svc, _ := enabledOIDC(t)
	deps := testDeps(&config.CLI{})
	deps.OIDC = svc
	router := NewRouter(deps)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/oidc/login" {
		t.Errorf("Location = %q, want /oidc/login", loc)
	}
}

func TestIndex_ServedWithValidSession(t *testing.T) {
	svc, ck := enabledOIDC(t)
	deps := testDeps(&config.CLI{})
	deps.OIDC = svc
	router := NewRouter(deps)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestIndex_PublicWhenOIDCDisabled(t *testing.T) {
	router := NewRouter(testDeps(&config.CLI{}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestDownloadPage_PublicWhenOIDCEnabled(t *testing.T) {
	svc, _ := enabledOIDC(t)
	deps := testDeps(&config.CLI{})
	deps.OIDC = svc

	// Seed a share so the download page renders fully.
	share := &domain.FileShare{
		ID:                "aabbccddeeff0011",
		Owner:             "owner-token",
		EncryptedMetadata: []byte("meta"),
		AuthKey:           []byte("auth"),
		Nonce:             []byte("nonce"),
		DLimit:            1,
		ExpireSeconds:     3600,
	}
	if err := deps.Meta.Set(context.Background(), share, 3600); err != nil {
		t.Fatalf("seed share: %v", err)
	}

	router := NewRouter(deps)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/download/aabbccddeeff0011", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (anonymous download page)", rec.Code)
	}
	if loc := rec.Header().Get("Location"); strings.Contains(loc, "/oidc/login") {
		t.Errorf("download page redirected to login: %q", loc)
	}
}

// wsDial connects to the test server's /api/ws with optional extra headers.
func wsDial(t *testing.T, srv *httptest.Server, hdr http.Header) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/ws"
	conn, _, err := websocket.DefaultDialer.Dial(url, hdr)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestUploadWS_Returns401FrameWithoutSession(t *testing.T) {
	svc, _ := enabledOIDC(t)
	deps := testDeps(&config.CLI{})
	deps.OIDC = svc
	srv := httptest.NewServer(NewRouter(deps))
	defer srv.Close()

	conn := wsDial(t, srv, nil)
	var resp map[string]int
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp["error"] != http.StatusUnauthorized {
		t.Errorf("response = %v, want {error: 401}", resp)
	}
}

func TestUploadWS_AllowedWithSessionCookie(t *testing.T) {
	svc, ck := enabledOIDC(t)
	deps := testDeps(&config.CLI{})
	deps.OIDC = svc
	srv := httptest.NewServer(NewRouter(deps))
	defer srv.Close()

	hdr := http.Header{"Cookie": []string{ck.String()}}
	conn := wsDial(t, srv, hdr)

	first, _ := json.Marshal(map[string]any{
		"fileMetadata":  crypto.EncodeB64([]byte("meta")),
		"authorization": "send-v1 " + crypto.EncodeB64([]byte("authkey")),
	})
	if err := conn.WriteMessage(websocket.TextMessage, first); err != nil {
		t.Fatalf("write first frame: %v", err)
	}
	var resp map[string]any
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if _, isErr := resp["error"]; isErr {
		t.Fatalf("got error response %v, want upload ack", resp)
	}
	if resp["url"] == "" || resp["id"] == "" || resp["ownerToken"] == "" {
		t.Errorf("incomplete ack: %v", resp)
	}
}

func TestUploadWS_CrossOriginRejected(t *testing.T) {
	svc, ck := enabledOIDC(t)
	deps := testDeps(&config.CLI{})
	deps.OIDC = svc
	srv := httptest.NewServer(NewRouter(deps))
	defer srv.Close()

	// Valid session, but the handshake claims a foreign origin — the CSRF
	// guard must reject it despite the cookie.
	hdr := http.Header{
		"Cookie": []string{ck.String()},
		"Origin": []string{"http://evil.example"},
	}
	conn := wsDial(t, srv, hdr)
	var resp map[string]int
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("read: %v", err)
	}
	if resp["error"] != http.StatusUnauthorized {
		t.Errorf("response = %v, want {error: 401}", resp)
	}
}

func TestUploadWS_OpenWhenOIDCDisabled(t *testing.T) {
	srv := httptest.NewServer(NewRouter(testDeps(&config.CLI{})))
	defer srv.Close()

	conn := wsDial(t, srv, nil)
	first, _ := json.Marshal(map[string]any{
		"fileMetadata":  crypto.EncodeB64([]byte("meta")),
		"authorization": "send-v1 " + crypto.EncodeB64([]byte("authkey")),
	})
	if err := conn.WriteMessage(websocket.TextMessage, first); err != nil {
		t.Fatalf("write first frame: %v", err)
	}
	var resp map[string]any
	if err := conn.ReadJSON(&resp); err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if _, isErr := resp["error"]; isErr {
		t.Fatalf("got error response %v, want upload ack (no auth configured)", resp)
	}
}
