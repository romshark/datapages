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

## Demo: Classifieds

This repository features a demo application resembling an online classifieds marketplace
under `example/classifieds`.
The code you'd write is in
[example/classifieds/app](https://github.com/romshark/datapages/tree/main/example/classifieds/app)
(the "source package").
The code that the generator produces is in
[example/classifieds/datapagesgen](https://github.com/romshark/datapages/tree/main/example/classifieds/datapagesgen).

To run the demo in development mode, use:

```sh
cd example/classifieds
make dev
```

You can then access:
- Preview: http://localhost:52000/
- Grafana Dashboards: http://localhost:3000/
- Prometheus UI: http://localhost:9091/

You can install [k6](https://k6.io/) and run `make load` in the background
to generate random traffic.
Increase the number of virtual users (`VU`) to apply more load to the server when needed.

To run the demo in production mode, use:

```sh
make stage
```

## Specification

See [SPECIFICATION.md](SPECIFICATION.md) for the full source package specification,
including handler signatures, parameters, return values, events, sessions, and modules.

## Development

### Prerequisites

- [Go](https://go.dev/dl/) (see version in `go.mod`)

### Contributing

See [CLAUDE.md](CLAUDE.md) for code style, testing
conventions, commit message format, and project structure.

Use the `example/classifieds/` application as a real-world
test fixture when developing Datapages.
