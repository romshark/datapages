# Datapages

[![CI](https://github.com/romshark/datapages/actions/workflows/ci.yml/badge.svg)](https://github.com/romshark/datapages/actions/workflows/ci.yml)
[![golangci-lint](https://github.com/romshark/datapages/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/romshark/datapages/actions/workflows/golangci-lint.yml)
[![Coverage Status](https://coveralls.io/repos/github/romshark/datapages/badge.svg?branch=main)](https://coveralls.io/github/romshark/datapages?branch=main)
[![Go Reference](https://pkg.go.dev/badge/github.com/romshark/datapages.svg)](https://pkg.go.dev/github.com/romshark/datapages)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
![Alpha](https://img.shields.io/badge/status-alpha-orange)

> [!WARNING]
> **Alpha Software:** Datapages is still in early development.
> APIs are subject to change and you may encounter bugs.

> [!NOTE]
> **v0.10 is coming** with API improvements, stateful pages,
> service worker support, multi-app modules and more!

A [Templ](https://templ.guide) + Go + [Datastar](https://data-star.dev) web framework
for building dynamic, server-rendered web applications in pure Go.

**Focus on your business logic, generate the boilerplate**
Datapages parses your app source package and generates all the wiring.
Routing, sessions and authentication, SSE streams, CSRF protection,
type-safe URL and action helpers, Prometheus metrics -
so your application code stays clean and takes full advantage of Go's strong
static typing and high performance.

No matter whether you're building **real-time collaborative dynamic web app**
or simple [HTMX](https://htmx.org/)-style websites - Datapages will serve you well.

## Examples

- [`counter`](example/counter/) — Real-time counter built twice in one module,
  bare bones as `app/simple` and polished as `app/fancy`. Bare bones starting
  point, and the example of a module that builds more than one application.
- [`todolist`](example/todolist/) — Real-time collaborative todo list with per-tab
  server-side state (Most [Tao](https://data-star.dev/guide/the_tao_of_datastar) conform example).
- [`calculator`](example/calculator/) — Hybrid calculator app that runs both as
  a multi-client server and single-client Desktop app.
- [`classifieds`](example/classifieds/) —
  Full-featured classifieds marketplace with sessions, auth, Prometheus metrics,
  Grafana dashboards and load testing.
- [`tailwindcss`](example/tailwindcss/) —
  Minimal static page demonstrating Tailwind CSS integration.
- [`webcomponents`](example/webcomponents/) —
  Landing page with vanilla and [Lit](https://lit.dev)-based Web Components
  bundled via esbuild through a custom watcher.
- [`sqlitesessions`](example/sqlitesessions/) —
  Custom `sessions.Manager` implementation backed by SQLite
  via [sqinn-go](https://github.com/cvilsmeier/sqinn-go) (no cgo).

## Getting Started

### Install

```sh
go install github.com/romshark/datapages/cmd/datapages@latest
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

Nothing about the build is configured. Every setting is read from the code
that already states it.

Static file serving is turned on by an `embed.FS` whose doc comment names the
URL path it is served at, the way a page names its route. The comment gives
the prefix, the `//go:embed` directive gives the directory:

```go
// app/assets.go

// StaticFS is /static/
//go:embed static/*
var StaticFS embed.FS
```

The app package, the session data type, the metrics mode and the package to
generate into are the type arguments of the `datapages.NewServer` call.

```go
s, err := datapages.NewServer[
	app.App,                     // App
	datapages.DisableSessions,   // SessionData
	datapages.DisablePrometheus, // Metrics
	datapagesgen.Server,         // S
](
	a, broker, datapages.WithLogger(logger),
)
```

`App` names the app package. `Metrics` decides the Prometheus counters:
`datapages.EnablePrometheus` generates the code that counts and requires
`WithPrometheus` to serve it, `datapages.DisablePrometheus` generates no
counters and rejects that option.
`S` must name that app package's `datapagesgen`, which is where its code is
generated:
`app/datapagesgen` for `./app`, `app/frontend/datapagesgen` for `./app/frontend`.

One module may build any number of applications. Each app package gets its own
model, its own generated package and its own entry point:

```
app/frontend/                 cmd/frontend/
app/frontend/datapagesgen/
app/admindashboard/           cmd/admindashboard/
app/admindashboard/datapagesgen/
```

`datapages gen` generates every one of them. `datapages watch` runs one, so a
module that builds more than one needs `--app` to say which:

```sh
datapages watch --app frontend
```

A module with no `NewServer` call yet is generated into `app/datapagesgen`
from `./app` and gets a `cmd/server/main.go` written for it.

What is left is the tooling, which `datapages.yaml` or `datapages.yml` in the
module root carries. If both files exist, the CLI treats that as an error.

```yaml
cmd: cmd/server
watch:
  exclude:
    - ".git/**" # git internals
    - ".*"      # hidden files/directories
    - "*~"      # editor backup files
```

These top-level keys are supported:

- `cmd`: path to the server command package. Default: `cmd/server`
- `watch`: development server settings

The URL path in the comment must start and end with `/` and cannot be `/`.
The `//go:embed` directive must name exactly one directory inside the app
package.

With `datapages.DisablePrometheus` the generated code has no Prometheus imports and
no metric variables. Use `datapages init --prometheus=false` to scaffold a
project whose entry point names it.

The optional `watch` section configures the development server
(host, proxy timeout, debounce, TLS, compiler flags, logging, custom watchers,
etc.).

## Specification

See [SPECIFICATION.md](SPECIFICATION.md) for the full source package specification,
including handler signatures, parameters, return values, events, sessions, and modules.

See [FAQ.md](FAQ.md) for frequently asked questions.

## Modules

Datapages ships pluggable modules with swappable implementations:

- [`Manager[Data]`](modules/sessions/sessions.go)
  - [`natskv`](https://pkg.go.dev/github.com/romshark/datapages/modules/sessions/natskv) - NATS KV store with AES-128-GCM encrypted cookies
  - [`inmem`](https://pkg.go.dev/github.com/romshark/datapages/modules/sessions/inmem) - In-memory sessions (lost on restart; single-instance only)
- [`Broker`](modules/messaging/messaging.go)
  - [`natscore`](https://pkg.go.dev/github.com/romshark/datapages/modules/messaging/natscore) - Core NATS backed message broker
  - [`inmem`](https://pkg.go.dev/github.com/romshark/datapages/modules/messaging/inmem) - In-memory fan-out message broker (single-instance only)
- [`TokenGenerator`, `TokenValidator`](modules/csrf/csrf.go)
  - [`hmac`](https://pkg.go.dev/github.com/romshark/datapages/modules/csrf/hmac) - HMAC-SHA256 with BREACH-resistant masking
- [`TokenGenerator`](modules/sessions/sessions.go)

## Motivation

The reason I built Datapages is that the combination of [Datastar](https://data-star.dev) + [Go](https://go.dev) + [Templ](https://templ.guide) is my preferred way of writing server-centric web applications. But in every project I used this tech stack for I kept repeating the same code patterns and solving the same problems over and over again. I realized many developers are repeating the same patterns too and struggle with the common pitfalls:

- How to handle SSE streams correctly?
- How to use NATS effectively?
- How to approach security and authentication?
- How to configure a convenient hot-reload for development?
- How to keep the code maintainable over time,
  especially when you add more developers and/or AI assistants?
- How to keep AI coding assistants from drifting too much?
- How to achieve optimal performance and a good UX for endusers?

Your Datastar frontends are your *rocket* to extraterrestrial worlds of the internet.
The further you want to go, the heavier a rocket you'll require. Hence, you need powerful boosters to get it off the ground and overcome Earth's gravity. Such boosters exist in the form of awesome templates like [zangster300/northstar](https://github.com/zangster300/northstar), which will quickly get your rocket to the stratosphere and beyond. But power alone is not enough — you also need good stabilizing fins and thrust vectoring to keep your rocket steady as it flies. I felt like this part was lacking in the Go ecosystem of Datastar. By enforcing a common structure of types, methods and other conventions with tooling, Datapages provides not only the power but also the stability your rocket needs to stay in flight for long and consume as little brain fuel as possible.

Not only does Datapages allow you to start quickly with `datapages init` and jump straight into building your application, but it also continuously supports you keeping accidental complexity low by:

- Providing a [Datastar Tao oriented](https://data-star.dev/guide/the_tao_of_datastar) architecture as a good default while preserving enough flexibility to go beyond if you need to.
- Providing `datapages gen` to generate all boilerplate code consistently and guide you and your AI coding agents.
- Providing `datapages lint` that can be used in CI/CD workflows for extensive static code analysis.
- Providing `datapages watch` to give you an interactive hot-reload environment for a fast feedback loop with error reporting directly in the browser preview.

Agentic coding is a big topic right now and likely here to stay. But LLMs tend to drift over time and introduce accidental complexity.
So for AI to be used more effectively I wanted to provide the skills and instructions necessary for agents to know how to deal with this tech stack and call into Datapages CLI help them when they drift by providing them with useful feedback.

## Who This Is For

Datapages is a good fit if you:

- **Already write your backend in Go** and want to build your web frontend
  in the same language and toolchain.
- **Are building a server-rendered application**, where the server owns the data;
  not a local-first offline-capable SPA.
- **Already use [Datastar](https://data-star.dev)** and want a Go framework
  to help you ship faster with less code while preserving maintainability.
- **Already use [Templ](https://templ.guide)** and want a full framework
  built around it.
- **Use [HTMX](https://htmx.org/),
  [idiomorph](https://htmx.org/extensions/idiomorph/)
  and [Alpine.js](https://alpinejs.dev/)**, and instead want a single cohesive stack
  with a smaller bundle size and less spaghetti-code.
- **Don't want to maintain a separate REST/GraphQL API** just to feed your frontend.
- **Want to deploy as a single, statically compiled binary** that makes
  the most of your hardware.
- **Want to develop hybrid desktop apps** in Go and HTML5 (see
  [Calculator example](https://github.com/romshark/datapages/tree/main/example/calculator))

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and commands, and
[AGENTS.md](AGENTS.md) for code style, testing conventions, commit message format
and project structure.

Use the `example/classifieds/` application as a real-world
test fixture when developing Datapages.
