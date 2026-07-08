# OIDC authentication (optional)

sendgo can gate **uploads** behind an OpenID Connect provider. The feature is
off by default: with no `--oidc-*` flags the server runs fully open, exactly
as before.

When enabled:

| Route | Behavior |
|---|---|
| `/` (upload page) | anonymous visitors are redirected to the provider |
| `/api/ws` (upload) | rejected without a valid session (`{"error": 401}`) |
| `/download/*`, `/api/download/*`, `/api/metadata/*`, `/api/exists/*` | **public** — receiving files never requires login |
| `/api/delete\|password\|params\|info` | unchanged (still gated by the share's `owner_token`) |
| `/oidc/login`, `/oidc/callback`, `/oidc/logout` | auth endpoints (registered only when OIDC is on) |

The flow is standard OIDC: discovery, authorization code flow with PKCE
(S256) and nonce verification. After login the server issues a stateless
HMAC-signed session cookie (`sendgo_session`) — no session store is needed.

## Flags

| Flag | Env | Default | Notes |
|---|---|---|---|
| `--oidc-issuer-url` | `OIDC_ISSUER_URL` | — | Enables auth. Must be set together with client id/secret. |
| `--oidc-client-id` | `OIDC_CLIENT_ID` | — | |
| `--oidc-client-secret` | `OIDC_CLIENT_SECRET` | — | |
| `--oidc-redirect-url` | `OIDC_REDIRECT_URL` | derived | Defaults to `<base-url>/oidc/callback` resolved per request. |
| `--oidc-scopes` | `OIDC_SCOPES` | `openid,profile,email` | |
| `--oidc-session-ttl` | `OIDC_SESSION_TTL` | `12h` | Session cookie lifetime. |
| `--oidc-cookie-secret` | `OIDC_COOKIE_SECRET` | random per start | **Set it in production** (min 32 chars): without it sessions are dropped on every restart, and multiple replicas can't validate each other's cookies. Generate with `openssl rand -hex 32`. |

Setting only some of issuer/client-id/client-secret is a startup error — a
typo can't silently disable auth.

## Example: Authentik

**Ready-made demo:** [examples/oidc-authentik](examples/oidc-authentik/) is a
self-contained `docker compose up -d` stack (Authentik + sendgo) where the
OAuth2 provider and application are provisioned automatically from a
[blueprint](examples/oidc-authentik/blueprints/sendgo.yaml). Manual setup
below.

1. In Authentik create an **OAuth2/OpenID Provider**:
   - Client type: *Confidential*
   - Redirect URI: `https://send.example.com/oidc/callback`
   - Signing key: any RS256 key
2. Create an **Application** bound to that provider and note the slug.
3. Run sendgo:

```sh
sendgo \
  --oidc-issuer-url=https://auth.example.com/application/o/<slug>/ \
  --oidc-client-id=<client id> \
  --oidc-client-secret=<client secret> \
  --oidc-cookie-secret=$(openssl rand -hex 32)
```

> **The trailing slash in the issuer URL matters.** go-oidc verifies the
> `iss` claim against the configured issuer byte-for-byte, and Authentik
> issuers end with `/`. Copy the "OpenID Configuration Issuer" value from the
> provider page verbatim.

## Reverse proxy notes

- The proxy **must** forward `X-Forwarded-Proto` (and `X-Forwarded-Host` if
  it rewrites hosts). sendgo uses them to build the `redirect_uri` and to
  decide whether session cookies get the `Secure` attribute. A missing
  `X-Forwarded-Proto: https` results in cookies without `Secure` and a
  mismatching redirect URI.
- Alternatively pin `--oidc-redirect-url` (and `--base-url`) explicitly.

## Semantics and edge cases

- **Downloads never require login** — recipients of a share link are not
  sent to the provider.
- The session is checked at the WebSocket **handshake** only: an upload in
  flight survives its session expiring mid-transfer.
- Cross-site WebSocket handshakes (foreign `Origin` header) are rejected
  while OIDC is on — the session cookie would otherwise make uploads
  CSRF-able. Non-browser clients that send no `Origin` header still work,
  but need a valid session cookie.
- CLI clients such as `ffsend` cannot log in interactively and therefore
  cannot upload while OIDC is enabled (downloading keeps working).
- Logout (`/oidc/logout`) clears the local session only; it does not end the
  provider session (no RP-initiated logout — the stateless cookie keeps no
  `id_token_hint`).
