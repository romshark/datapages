# Datapages

[![CI](https://github.com/romshark/datapages/actions/workflows/ci.yml/badge.svg)](https://github.com/romshark/datapages/actions/workflows/ci.yml)
[![golangci-lint](https://github.com/romshark/datapages/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/romshark/datapages/actions/workflows/golangci-lint.yml)
[![Coverage Status](https://coveralls.io/repos/github/romshark/datapages/badge.svg?branch=main)](https://coveralls.io/github/romshark/datapages?branch=main)
[![Go Report Card](https://goreportcard.com/badge/github.com/romshark/datapages)](https://goreportcard.com/report/github.com/romshark/datapages)
[![Go Reference](https://pkg.go.dev/badge/github.com/romshark/datapages.svg)](https://pkg.go.dev/github.com/romshark/datapages)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Alpha](https://img.shields.io/badge/status-alpha-orange)

> **🧪 Alpha Software:** Datapages is still in early development 🚧.<br>
> APIs are subject to change and you may encounter bugs.

A [Templ](https://templ.guide) + Go + [Datastar](https://data-star.dev) web framework
for building dynamic, server-rendered web applications in pure Go.

**Focus on your business logic, generate the boilerplate**
Datapages parses your app source package and generates all the wiring.
Routing, sessions and authentication, SSE streams, CSRF protection,
type-safe URL and action helpers -
so your application code stays clean and takes full advantage of Go's strong
static typing and high performance.

## Examples

- [`counter`](example/counter/) — Minimal real-time counter. Bare bones starting point.
- [`fancy-counter`](example/fancy-counter/) — Fancy real-time collaborative counter.
- [`classifieds`](example/classifieds/) — Full-featured classifieds marketplace with sessions, auth, Prometheus metrics, and load testing.
- [`tailwindcss`](example/tailwindcss/) — Minimal static page demonstrating Tailwind CSS integration.

## Getting Started

### Install

```sh
go install github.com/romshark/datapages@latest
```

### Initialize New Project

```sh
datapages init
```

## CLI Commands

| Command             | Description                                                  |
| ------------------- | ------------------------------------------------------------ |
| `datapages init`    | Initialize a new project with scaffolding and configuration. |
| `datapages gen`     | Parse the app model and generate the datapages package.      |
| `datapages watch`   | Start the live-reloading development server.                 |
| `datapages lint`    | Validate the app model without generating code.              |
| `datapages version` | Print CLI version information.                               |

## Configuration

Datapages reads configuration from `datapages.yaml` in the module root:

```yaml
app: app            # Path to the app source package (default)
gen:
  package: datapagesgen # Path to the generated package (default)
  prometheus: true      # Enable Prometheus metrics generation (default)
cmd: cmd/server     # Path to the server cmd package (default)
```

When `prometheus` is set to `false`, the generated server code will not include
Prometheus imports, metric variables, or the `WithPrometheus` server option.
Use `datapages init --prometheus=false` to scaffold a project without Prometheus.

The optional `watch` section configures the development server
(host, proxy timeout, TLS, compiler flags, custom watchers, etc.).

## Specification

See [SPECIFICATION.md](SPECIFICATION.md) for the full source package specification,
including handler signatures, parameters, return values, events, sessions, and modules.

## Technical Limitations

- For now, with CSRF protection enabled, you will not be able to use plain HTML forms,
  since the CSRF token is auto-injected for Datastar `fetch` requests
  (where `Datastar-Request` header is `true`).
  You must use Datastar actions for any sort of server interactivity.

## Modules

Datapages ships pluggable modules with swappable implementations:

- [`SessionManager[S]`](modules/sessmanager/sessmanager.go)
  - [`natskv`](https://pkg.go.dev/github.com/romshark/datapages/modules/sessmanager/natskv) - NATS KV store with AES-128-GCM encrypted cookies
  - [`inmem`](https://pkg.go.dev/github.com/romshark/datapages/modules/sessmanager/inmem) - In-memory sessions (lost on restart; single-instance only)
- [`MessageBroker`](modules/msgbroker/msgbroker.go)
  - [`natsjs`](https://pkg.go.dev/github.com/romshark/datapages/modules/msgbroker/natsjs) - NATS JetStream backed message broker
  - [`inmem`](https://pkg.go.dev/github.com/romshark/datapages/modules/msgbroker/inmem) - In-memory fan-out message broker (single-instance only)
- [`TokenManager`](modules/csrf/csrf.go)
  - [`hmac`](https://pkg.go.dev/github.com/romshark/datapages/modules/csrf/hmac) - HMAC-SHA256 with BREACH-resistant masking
- [`TokenGenerator`](modules/sessmanager/sessmanager.go)
  - [`sesstokgen`](https://pkg.go.dev/github.com/romshark/datapages/modules/sesstokgen) - Cryptographically random session tokens (256-bit)

## Development

### Prerequisites

- [Go](https://go.dev/dl/) (see version in `go.mod`)

### Contributing

See [CLAUDE.md](CLAUDE.md) for code style, testing
conventions, commit message format, and project structure.

Use the `example/classifieds/` application as a real-world
test fixture when developing Datapages.
