# [![Send](./assets/icon-64x64.png)](https://gitlab.com/timvisee/send/) Send

[![Build status on GitLab CI][gitlab-ci-master-badge]][gitlab-ci-link]
[![Latest release][release-badge]][release-link]
[![Docker image][docker-image-badge]][docker-image-link]
[![Project license][repo-license-badge]](LICENSE)

[docker-image-badge]: https://img.shields.io/badge/docker-latest-blue.svg
[docker-image-link]: https://gitlab.com/timvisee/send/container_registry/eyJuYW1lIjoidGltdmlzZWUvc2VuZCIsInRhZ3NfcGF0aCI6Ii90aW12aXNlZS9zZW5kL3JlZ2lzdHJ5L3JlcG9zaXRvcnkvMTQxODUwNC90YWdzP2Zvcm1hdD1qc29uIiwiaWQiOjE0MTg1MDQsImNsZWFudXBfcG9saWN5X3N0YXJ0ZWRfYXQiOm51bGx9
[gitlab-ci-link]: https://gitlab.com/timvisee/send/pipelines
[gitlab-ci-master-badge]: https://gitlab.com/timvisee/send/badges/master/pipeline.svg
[release-badge]: https://img.shields.io/github/v/tag/timvisee/send
[release-link]: https://gitlab.com/timvisee/send/-/tags
[repo-license-badge]: https://img.shields.io/github/license/timvisee/send.svg

A fork of Mozilla's [Firefox Send][mozilla-send].
Mozilla discontinued Send, this fork is a community effort to keep the project
up-to-date and alive.

- Forked [at][fork-commit] Mozilla's last publicly hosted version
- _Mozilla_ & _Firefox_ branding [is][remove-branding-pr] removed so you can legally self-host
- Kept compatible with [`ffsend`][ffsend] (CLI for Send)
- Dependencies have been updated
- Mozilla's [changes][mozilla-patches] since the fork have been selectively [merged][mozilla-patches-pr]
- Mozilla's experimental report feature, download tokens, trust warnings and FxA changes are not included
- **Backend rewritten in Go** — single self-contained binary, optional Redis/S3,
  in-process cleanup of orphaned blobs. The public API (URLs, HMAC+nonce auth,
  WebSocket upload protocol, `/config` JSON) is 1:1 with the upstream Node
  server, so the frontend is unchanged.

Find an up-to-date Docker image here: [docs/docker.md](docs/docker.md)

The original project by Mozilla can be found [here][mozilla-send].
The [`mozilla-master`][branch-mozilla-master] branch holds the `master` branch
as left by Mozilla.
The [`send-v3`][branch-send-v3] branch holds the commit tree of Mozilla's last
publicly hosted version, which this fork is based on.
The [`send-v4`][branch-send-v4] branch holds the commit tree of Mozilla's last
experimental version which was still a work in progress (featuring file
reporting, download tokens, trust warnings and FxA changes), this has
selectively been merged into this fork.
Please consider to [donate][donate] to allow me to keep working on this.

Thanks [Mozilla][mozilla] for building this amazing tool!

[branch-mozilla-master]: https://gitlab.com/timvisee/send/-/tree/mozilla-master
[branch-send-v3]: https://gitlab.com/timvisee/send/-/tree/send-v3
[branch-send-v4]: https://gitlab.com/timvisee/send/-/tree/send-v4
[donate]: https://timvisee.com/donate
[ffsend]: https://github.com/timvisee/ffsend
[fork-commit]: https://gitlab.com/timvisee/send/-/commit/3e9be676413a6e1baaf6a354c180e91899d10bec
[mozilla-patches-pr]: https://gitlab.com/timvisee/send/-/merge_requests/3
[mozilla-patches]: https://gitlab.com/timvisee/send/-/compare/3e9be676413a6e1baaf6a354c180e91899d10bec...mozilla-master
[mozilla-send]: https://github.com/mozilla/send
[mozilla]: https://mozilla.org/
[remove-branding-pr]: https://gitlab.com/timvisee/send/-/merge_requests/2

---

**Docs:** [FAQ](docs/faq.md), [Encryption](docs/encryption.md), [Build](docs/build.md), [Docker](docs/docker.md), [More](docs/)

---

## Table of Contents

* [What it does](#what-it-does)
* [Requirements](#requirements)
* [Development](#development)
* [Commands](#commands)
* [Configuration](#configuration)
* [Localization](#localization)
* [Contributing](#contributing)
* [Instances](#instances)
* [Deployment](#deployment)
* [Clients](#clients)
* [License](#license)

---

## What it does

A file sharing experiment which allows you to send encrypted files to other users.

---

## Requirements

- [Go 1.23+](https://go.dev/) (to build the backend)
- [Node.js 16.x](https://nodejs.org/) (to build the frontend bundle)
- [`just`](https://github.com/casey/just) (`brew install just`) for the dev recipes
- [Redis server](https://redis.io/) (optional — in-memory store is used by default)
- [S3-compatible object storage](https://aws.amazon.com/s3/), e.g. Yandex Object
  Storage (optional — local filesystem is used by default)

---

## Development

To start an ephemeral development server, run:

```sh
just dev
```

Then, browse to http://localhost:1443

`just dev` builds the frontend with webpack, copies it into `server/static/`
for `//go:embed`, and runs the Go server with sane defaults (in-memory
metadata, local FS blob storage, cleanup goroutine on).

---

## Commands

| Command          | Description |
|------------------|-------------|
| `npm run format` | Formats the frontend code using **prettier**.
| `npm run lint`   | Lints the CSS and JavaScript code.
| `just test`      | Runs the Go unit tests (`go test -race`).
| `just dev`       | Runs the server in development configuration.
| `just build`     | Builds the frontend bundle and the Go binary into `bin/sendgo`.
| `just run`       | Runs the compiled binary with development defaults; extra flags are forwarded (`just run --redis-dsn=redis://localhost:6379/0`).
| `just run-prod`  | Same as `just run` but with json logs and `--file-dir=./var/blobs`.

---

## Configuration

The server is configured with CLI flags or environment variables. Every flag
has an environment-variable equivalent; the env names match the upstream
Node server so existing `.env` files keep working. Flag priority:
**flag > env > default**.

See `./bin/sendgo --help` for the full list (server, limits, meta store,
blob storage, cleanup, observability, branding).

---

## Localization

See: [docs/localization.md](docs/localization.md)

---

## Contributing

Pull requests are always welcome! Feel free to check out the list of "good first issues" (to be implemented).

---

## Instances

Find a list of public instances here: https://github.com/timvisee/send-instances/

---

## Deployment

See: [docs/deployment.md](docs/deployment.md)

Docker quickstart: [docs/docker.md](docs/docker.md)

AWS example using Ubuntu Server `20.04`: [docs/AWS.md](docs/AWS.md)

---

## Clients

- Web: _this repository_
- Command-line: [`ffsend`](https://github.com/timvisee/ffsend)
- Android: _see [Android](#android) section_
- Thunderbird: [FileLink provider for Send](https://addons.thunderbird.net/thunderbird/addon/filelink-provider-for-send/)

#### Android

The android implementation is contained in the `android` directory,
and can be viewed locally for easy testing and editing by running `ANDROID=1 npm
start` and then visiting <http://localhost:8080>. CSS and image files are
located in the `android/app/src/main/assets` directory.

---

## License

[Mozilla Public License Version 2.0](LICENSE)

[qrcode.js](https://github.com/kazuhikoarase/qrcode-generator) licensed under MIT

---
