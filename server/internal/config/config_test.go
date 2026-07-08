package config

import (
	"testing"
	"time"
)

func TestLoad_OIDCDefaults(t *testing.T) {
	cfg := Load([]string{})
	if cfg.OIDCIssuerURL != "" || cfg.OIDCClientID != "" || cfg.OIDCClientSecret != "" {
		t.Errorf("OIDC flags must default to empty (auth disabled), got issuer=%q id=%q",
			cfg.OIDCIssuerURL, cfg.OIDCClientID)
	}
	wantScopes := []string{"openid", "profile", "email"}
	if len(cfg.OIDCScopes) != len(wantScopes) {
		t.Fatalf("OIDCScopes = %v, want %v", cfg.OIDCScopes, wantScopes)
	}
	for i, s := range wantScopes {
		if cfg.OIDCScopes[i] != s {
			t.Errorf("OIDCScopes[%d] = %q, want %q", i, cfg.OIDCScopes[i], s)
		}
	}
	if cfg.OIDCSessionTTL != 12*time.Hour {
		t.Errorf("OIDCSessionTTL = %v, want 12h", cfg.OIDCSessionTTL)
	}
}

func TestLoad_OIDCFlagsParsed(t *testing.T) {
	cfg := Load([]string{
		"--oidc-issuer-url=https://idp.example.com/app/",
		"--oidc-client-id=sendgo",
		"--oidc-client-secret=s3cret",
		"--oidc-session-ttl=1h",
	})
	if cfg.OIDCIssuerURL != "https://idp.example.com/app/" {
		t.Errorf("OIDCIssuerURL = %q", cfg.OIDCIssuerURL)
	}
	if cfg.OIDCClientID != "sendgo" || cfg.OIDCClientSecret != "s3cret" {
		t.Errorf("client id/secret not parsed: %q/%q", cfg.OIDCClientID, cfg.OIDCClientSecret)
	}
	if cfg.OIDCSessionTTL != time.Hour {
		t.Errorf("OIDCSessionTTL = %v, want 1h", cfg.OIDCSessionTTL)
	}
}
