# AGENTS.md

Guidance for AI agents working with this repository.

---

## Operating rules (read first)

1. **Do not commit or push without explicit user instruction.**
   Any command of the form `git commit`, `git push`, `git tag`, `git merge`,
   `git rebase --commit`, and any operation that mutates history or the remote
   branch — **only on direct request** ("commit it", "push it", "tag it").
   Implicit permission inferred from context does not count.

2. **All git operations run under the user's identity.**
   - Do not use `--author=…`; do not add `Co-Authored-By: Claude/Anthropic/AI`
     to commit messages; do not insert "🤖 Generated with…"; do not mention an
     AI / agent / model in `git log`, PR descriptions, or commit bodies.
   - `user.name` and `user.email` are taken from the user's local git config;
     do not overwrite them.

3. **No destructive operations without confirmation.**
   `git reset --hard`, `git push --force`, `git checkout -- .`,
   `rm -rf` against existing user-code directories, `gh repo delete`, and
   similar — only after explicit request.

4. **System changes outside the repository** (installing system packages,
   editing `$HOME/.zshrc`, global `npm i -g`, etc.) — only on explicit request.

5. **All documentation in this repository is in English.**
   This includes `README.md`, `AGENTS.md`, files under `docs/`, and any new
   Markdown file. Do not author Russian (or other non-English) docs. Existing
   Russian comments inside source files are kept as-is until an explicit
   migration is requested; new code comments should be in English.

6. **Commit and branch naming — Conventional Commits 1.0.0**
   (<https://www.conventionalcommits.org/en/v1.0.0/>).

   **Commit format:**
   ```
   <type>[optional scope]: <description>

   [optional body]

   [optional footer(s)]
   ```

   **Allowed types** (minimal set; extend only with prior agreement):
   - `feat` — new feature (`MINOR` per semver)
   - `fix` — bug fix (`PATCH` per semver)
   - `docs` — documentation only
   - `style` — formatting, whitespace, no behavior change
   - `refactor` — code change that is neither a feature nor a fix
   - `perf` — performance improvement
   - `test` — adding or fixing tests
   - `build` — build files (Dockerfile, justfile, go.mod, package.json)
   - `ci` — CI configuration
   - `chore` — routine tasks (version bumps, README touch-ups, etc.)
   - `revert` — reverting a previous commit

   **Scope** is optional but encouraged. Pick from paths under
   `server/internal/` or from logical domains: `api`, `domain`, `usecase`,
   `storage`, `meta`, `http`, `config`, `crypto`, `i18n`, `cleanup`,
   `observability`, `frontend`, `docker`.

   **Breaking changes** — `!` after type/scope OR a `BREAKING CHANGE:` footer:
   - `feat(api)!: rename /api/exists to /api/shares/:id`
   - or in footer: `BREAKING CHANGE: removes /api/filelist endpoints`

   **Examples:**
   ```
   feat(storage): add S3 multipart for files >5GB
   fix(http): preserve nonce on 401 responses
   refactor(meta): extract cleanup index into separate file
   docs(agents): add conventional-commits rule
   build(docker): switch base to distroless:nonroot
   chore: bump go-redis to v9.19
   feat(auth)!: require OIDC for download endpoints
   ```

   **Branch naming** uses the same type prefixes, separated by `/`:
   `feat/oidc-integration`, `fix/cleanup-race`, `docs/readme-yandex`,
   `refactor/split-router`, `chore/deps-bump`. Use `-` for word separation;
   no underscores or spaces.

   Commit subjects in English. Bodies and footers in English as well, to
   match the project's `git log`.

---

## About the project

**sendgo** is a fork of Mozilla Firefox Send / Timvisee Send. It is an
end-to-end encrypted file sharing service. Encryption happens **on the
client** (AES-GCM through ECE framing, HKDF-SHA-256). The server only ever
sees ciphertext plus opaque encrypted metadata. The share secret lives in
the URL `#` fragment and **never** reaches the server.

**The crypto model is the product. Do not break it.**

### Current stack

| Layer         | Technology |
|---------------|------------|
| Frontend      | Existing JS (webpack, choo, Fluent for i18n) — frozen |
| Backend       | Go 1.23+ (previously Node.js, fully replaced) |
| Storage       | S3 (optional) or local filesystem (default) |
| Metadata      | Redis (optional) or in-memory map (default) |
| Auth          | HMAC + nonce challenge (1:1 with legacy Node contract) |
| Observability | `log/slog` (stdlib) + optional Sentry |

### What was removed and must NOT be re-added without discussion

- **Node.js backend** — replaced by Go code under `server/`.
- **FXA (Firefox Accounts)** — defunct Mozilla service. `/api/filelist/*`
  endpoints are gone; `FXA_*` environment variables are ignored.
- **GCS storage adapter** — only S3 + FS remain.
- **BadgerDB and other embedded KV stores** — only Redis + in-memory map.
- **`docker-compose.yml`** — the binary is self-contained; no compose file
  lives in the repo.
- **PostgreSQL** — metadata lives in Redis or memory only, no SQL store.

---

## Repository layout

```
sendgo/
├── AGENTS.md                  ← you are here
├── README.md
├── Dockerfile                 multi-stage: webpack + Go build → distroless
├── justfile                   all development tasks
├── app/                       FROZEN: legacy JS frontend; change only on request
├── common/, public/, assets/  ↓ frontend statics, also frozen
├── webpack.config.js          ↓ frontend build pipeline
├── package.json               frontend-only deps; backend deps removed
└── server/                    Go backend (see below)
```

### server/ — Go backend (DDD-lite)

```
server/
├── go.mod
├── cmd/sendgo/main.go         entry point: kong CLI + bootstrap + graceful shutdown
├── static/                    embed-FS for dist/ and locales/ (populated by prepare-static)
└── internal/
    ├── domain/                entities, value objects, domain errors
    ├── port/                  interfaces: repositories (MetaStore) + clients (BlobStorage, Clock, TokenGenerator)
    ├── usecase/               application use cases (one file per use case)
    ├── crypto/                HMAC, token generation
    ├── config/                kong CLI struct (see `sendgo --help`)
    ├── i18n/                  locale negotiation for <html lang>
    └── adapter/
        ├── http/              chi.Mux, handlers/, middleware/, static.go, pages.go
        ├── storage/           FS, S3, factory
        ├── meta/              in-memory, Redis, factory
        ├── cleanup/           in-process goroutine that deletes orphaned blobs
        └── observability/     slog + ReopenableWriter (SIGHUP reopen) + Sentry
```

### Go dependencies

Minimum-frameworks policy:

- `net/http` + `github.com/go-chi/chi/v5` — routing + middleware
- `github.com/gorilla/websocket` — WS upload
- `github.com/aws/aws-sdk-go-v2/*` — S3
- `github.com/redis/go-redis/v9` — Redis client
- `github.com/alecthomas/kong` — CLI / env config
- `github.com/getsentry/sentry-go` — Sentry
- `log/slog` (stdlib) — logging

**Before adding a new dependency, check if the standard library suffices.**

---

## Public API contract — 1:1 with legacy Node

The public HTTP endpoints (URLs, headers, JSON schemas, WS protocol) are
**byte-identical to the original Node server**, so that the existing
frontend `app/api.js` keeps working without edits.

Changes to the public contract (URLs, request/response shapes, HMAC
protocol) — **only after explicit discussion**. Internal layout (package
structure, function names, middleware additions) is fair game.

| Method | Path | Auth | Source |
|--------|------|------|--------|
| GET    | `/` | — | `internal/adapter/http/pages.go` |
| GET    | `/config` | — | `handlers/config.go` |
| GET    | `/dist/*`, `/locales/*` | — | `static.go` |
| GET    | `/api/exists/:id` | — | `handlers/exists.go` |
| GET    | `/api/metadata/:id` | HMAC+nonce | `handlers/metadata.go` |
| GET    | `/api/download/:id`, `/api/download/blob/:id` | HMAC+nonce | `handlers/download.go` |
| POST   | `/api/delete/:id` | owner_token (body) | `handlers/delete.go` |
| POST   | `/api/password/:id` | owner_token (body) | `handlers/password.go` |
| POST   | `/api/params/:id` | owner_token (body) | `handlers/params.go` |
| POST   | `/api/info/:id` | owner_token (body) | `handlers/info.go` |
| WS     | `/api/ws` | — | `handlers/upload_ws.go` |
| GET    | `/__heartbeat__`, `/__lbheartbeat__`, `/__version__` | — | `handlers/health.go` |

WS upload protocol: first JSON frame with metadata → ack frame with
`{url, ownerToken, id}` → binary frames → EOF frame of length 1 with byte
`0x00` → ack `{ok: true}`.

---

## Build and run

All tasks are wrapped in `just`. Install with `brew install just`.

```bash
just                          # list recipes
just dev                      # quick `go run` cycle, in-memory + FS
just dev --log-level=debug    # extra flags are forwarded
just run                      # run the built binary
just run --redis-dsn=redis://localhost:6379/0
just run --s3-bucket=mybucket --s3-endpoint=https://storage.yandexcloud.net
just build                    # webpack + go build
just test                     # Go unit tests
just vet                      # go vet
just help                     # binary --help output
```

The binary is self-contained — with defaults it starts without any
external services (in-memory meta + FS blob in `./tmp/blobs`).

### Yandex Object Storage (target deployment)

```bash
AWS_ACCESS_KEY_ID=...     \
AWS_SECRET_ACCESS_KEY=... \
just run-prod \
    --s3-bucket=sendgo \
    --s3-endpoint=https://storage.yandexcloud.net \
    --s3-region=ru-central1 \
    --redis-dsn=redis://... \
    --log-file=/var/log/sendgo.log
```

---

## Coding rules

1. **The public endpoint contract is sacred.** Any change to URLs, JSON
   shapes, headers, the HMAC protocol, or the WS protocol — only after
   discussing with the user. Internal structure (private API, function
   names, package splits) is free to change.

2. **No cyclic imports between layers.**
   `domain → port` is the only allowed dependency for the domain.
   `usecase → domain + port` is the only allowed dependency for use cases.
   `adapter/* → domain + port + (optionally) usecase`.
   `cmd → everything`.

3. **Use cases must not know about HTTP.** No `*http.Request` /
   `*http.ResponseWriter` / chi inside `internal/usecase/`.

4. **Adapters implement ports.** Any external system (Redis, S3, the
   filesystem, a future OIDC provider) sits behind an interface from
   `internal/port/`.

5. **All CLI/env parameters go through kong**, in
   `internal/config/config.go`. Avoid scattering `os.Getenv("…")` calls
   throughout the code.

6. **Logging uses stdlib `slog`.** No third-party loggers. Carry the logger
   through `context.Context` (see `observability.WithLogger` /
   `FromContext`).

7. **Tests accompany new code.** Add at least one `_test.go` next to any
   new use case or adapter. Run with `just test` (which uses `-race`). See
   rules 12–15 for the unit-vs-integration split, fakes, and coverage gates.

8. **Code comments.** English is preferred for new code. Existing Russian
   comments are kept as-is until an explicit migration is requested.

9. **Minimum frameworks.** Before adding a dependency, check the standard
   library.

10. **`go vet ./...` and `go test -race ./...` must be green** before
    proposing any commit.

11. **Use cases must not touch external systems directly.**
    Code under `internal/usecase/` and `internal/domain/` may import only:
    the Go standard library (excluding `net/http`, `database/sql`, and
    filesystem-write APIs); `internal/domain` and `internal/port`;
    `internal/config` (the kong struct, read-only). Anything network-,
    disk-, time-, or randomness-bearing arrives through a `port.*`
    interface — either a **repository** (entity-aware persistence; e.g.
    `MetaStore` knows about `FileShare`) or a **client** (generic external
    service; e.g. `BlobStorage` for opaque bytes, `Clock` for time,
    `TokenGenerator` for randomness). Both kinds are plain Go interfaces
    that live in `internal/port/`. The `depguard` ruleset in
    `server/.golangci.yml` enforces this — adding a banned import fails
    `just vet` / `golangci-lint run`.

12. **Use cases are unit-tested against fakes.**
    Every file in `internal/usecase/` ships with a `*_test.go` next to it.
    Tests use hand-written fakes from `internal/port/portfakes/` (see rule
    13). The package `internal/usecase/...` is held at **100% line
    coverage** by `just test-usecase`; PRs that drop the number are
    rejected. Coverage of `internal/domain/` is not gated but should grow
    alongside any new domain logic.

13. **Test doubles live in `internal/port/portfakes/`.**
    One file per port: `fake_<port>.go`. Type name: `Fake<Interface>`.
    Each file ends with a compile-time check
    `var _ port.X = (*FakeX)(nil)`. Fakes are deterministic in-memory
    state machines with optional `ErrOn<Method>` error hooks and
    `<Method>Calls` recorders. They are NOT strict mocks — assertions
    live in the test, not inside the fake. Production code MUST NOT
    import `portfakes`; depguard enforces this.

14. **Test taxonomy.**
    Unit tests (`*_test.go`, no build tag, colocated with the code) must
    not touch the network, disk (except `t.TempDir()`), or `time.Now()`.
    Integration tests live under `server/test/integration/...` with
    `//go:build integration` and are run via `just itest`; they are not
    subject to the 100% coverage gate.

15. **Coverage gate.**
    `just test-usecase` runs the use-case package with
    `-coverpkg=./internal/usecase/...` and fails on any function whose
    coverage is not exactly `100.0%`. When CI is wired up, run this
    recipe alongside `just lint`.

16. **The backend is payload-agnostic.**
    Client-supplied encrypted material — `EncryptedMetadata`, `AuthKey`,
    and the blob stream flowing through `BlobStorage` — is opaque to the
    server. `internal/domain/`, `internal/port/`, and `internal/usecase/`
    treat these values strictly as `[]byte` / `io.Reader` pass-through.

    - **No decryption.** `crypto/aes`, `crypto/cipher`, and any
      cipher-suite package have no place in core. HMAC verification of
      the auth challenge happens in
      `adapter/http/middleware/auth_hmac.go`, not in a use case; the use
      case receives an already-authenticated `*domain.FileShare`.
    - **No interpretation.** Nothing in core may JSON/CBOR-decode
      `EncryptedMetadata`, sniff blob headers, or branch on payload
      content. A use case may read the byte count via `io.LimitReader` +
      a counter wrapper (see `stream_upload.go`), but must never buffer
      the blob whole, hash it, or compare bytes.
    - **No encoding work.** Base64 ↔ `[]byte` conversion lives in
      `adapter/http` (`handlers/upload_ws.go`, `handlers/metadata.go`).
      Use cases receive already-decoded bytes; ports return raw bytes.
    - **No alternate exit.** The blob always enters and leaves through
      `port.BlobStorage.Put` / `Get`. Direct `os.WriteFile`,
      `os.Create`, or other stdlib byte sinks inside core are forbidden
      — they route user content around the abstraction.

    **Why this is structural.** The whole security model rests on the
    server being unable to read user content (see `docs/encryption.md`).
    A "convenience" that base64-decodes metadata to log a filename,
    imports `crypto/aes` in a use case "just to peek", or buffers a blob
    for inspection silently breaks that promise without failing a test.
    When adding code in `internal/domain/`, `internal/port/`, or
    `internal/usecase/`, ask: "is this byte-stream plumbing, or am I
    looking at client content?" — if the latter, it belongs in
    `adapter/http` or not in the repo at all.

---

## What to do about webpack warnings

`npm run build` prints many warnings from deprecated dependencies in the
legacy frontend. That is **out of scope** (frontend is frozen). Ignore
them until the user explicitly requests a frontend upgrade.

---

## What NOT to do without explicit request

- Do not modify `app/`, `common/`, `public/`, `assets/`,
  `webpack.config.js`, `package.json` (backend deps have already been
  removed from it).
- Do not bring back the GCS adapter, BadgerDB store, FXA endpoints,
  `docker-compose.yml`, or PostgreSQL — they were removed deliberately.
- Do not change the Redis key schema (it matches legacy Node 1:1 so that
  hot migrations remain possible).
- Do not change the `/api/ws` message format (the frontend will break).
- Do not rename existing environment variables (e.g. `MAX_FILE_SIZE`); the
  goal is that existing `.env` files keep working.
- Do not commit, push, or open a PR without an explicit request.
- Do not mention AI / Claude / Anthropic in commit messages, PR
  descriptions, CHANGELOG, README, or source files.

---

## Useful locations for an agent

- Architecture rewrite plan: `~/.claude/plans/buzzing-yawning-axolotl.md`
- Entry point: `server/cmd/sendgo/main.go`
- DI wiring (all use cases and adapters are assembled here): same `main.go`
- CLI definition: `server/internal/config/config.go`
- HTTP router: `server/internal/adapter/http/router.go`
- Domain error → HTTP status mapping:
  `server/internal/adapter/http/handlers/problem.go`
