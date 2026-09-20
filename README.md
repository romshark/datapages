<p>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/logo_full_dark.svg">
    <img alt="Datapages" src="docs/logo_full.svg" width="480">
  </picture>
</p>

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

A [Templ](https://templ.guide) + Go + [Datastar](https://data-star.dev) web framework for building dynamic, server-rendered web applications in pure Go.

**Focus on your business logic, generate the boilerplate** Datapages parses your app source package and generates all the wiring. Routing, sessions and authentication, SSE streams, per-tab server-side state, CSRF protection, type-safe URL and action helpers, Prometheus metrics - so your application code stays clean and takes full advantage of Go's strong static typing and high performance.

No matter whether you're building **real-time collaborative dynamic web app** or simple [HTMX](https://htmx.org/)-style websites - Datapages will serve you well.

## Examples

- [`counter`](example/counter/): Real-time counter built twice in one module, bare bones as `app/simple` and polished as `app/fancy`. Bare bones starting point, and the example of a module that builds more than one application.
- [`todolist`](example/todolist/): Real-time collaborative todo list with per-tab server-side state (Most [Tao](https://data-star.dev/guide/the_tao_of_datastar) conform example).
- [`calculator`](example/calculator/): Hybrid calculator app that runs both as a multi-client server and single-client Desktop app.
- [`classifieds`](example/classifieds/): Full-featured classifieds marketplace with sessions, auth, Prometheus metrics, Grafana dashboards and load testing.
- [`tailwindcss`](example/tailwindcss/): Minimal static page demonstrating Tailwind CSS integration.
- [`webcomponents`](example/webcomponents/): Landing page with vanilla and [Lit](https://lit.dev)-based Web Components bundled via esbuild through a custom watcher.
- [`sqlitesessions`](example/sqlitesessions/): Custom `sessions.Manager` implementation backed by SQLite via [sqinn-go](https://github.com/cvilsmeier/sqinn-go) (no cgo).
- [`fast-shim`](example/fast-shim/): A cached placeholder shown immediately and replaced with the live response.
- [`offline-cache`](example/offline-cache/): Service worker support with handler-cached pages, a `PageOffline` fallback, and tickets available without a connection.

## Getting Started

### Install

```sh
go install github.com/romshark/datapages/cmd/datapages@latest
```

### Initialize New Project

```sh
datapages init
```

### AI Coding Agent Instructions

`datapages init` also writes the instructions AI coding agents read:

| path | purpose |
| ---- | ------- |
| `AGENTS.md` | project instructions for agents that follow the [AGENTS.md](https://agents.md) convention |
| `CLAUDE.md` | points Claude Code to `AGENTS.md` |
| `GEMINI.md` | points Gemini CLI to `AGENTS.md` |
| `.github/copilot-instructions.md` | points GitHub Copilot to `AGENTS.md` |
| `.cursor/rules/datapages.mdc` | points Cursor to `AGENTS.md` |
| `.agents/skills/*/SKILL.md` | task guides for coding agents |
| `.claude/skills/*/SKILL.md` | the same task guides for Claude Code |

The guides cover architecture decisions, Datapages rules, pages, actions, events, per-tab state, sessions, server setup, templates and Datastar. `datapages init` writes the same skills to both directories. Claude Code reads `.claude/skills`; `AGENTS.md` points other agents to `.agents/skills`. Manual edits do not sync between the directories. Edit both copies when you use both.

Edit the files as needed. Run `datapages init -n` in an existing project to install the CLI's current instructions. If a file differs, init saves the prior copy beside it with a `.bak` suffix and adds a number if that name exists. Pass `--no-ai-skills` to skip agent instructions.

## CLI Commands

| Command             | Description                                                  |
| ------------------- | ------------------------------------------------------------ |
| `datapages init`    | Initialize a new project with scaffolding and configuration. |
| `datapages gen`     | Parse the app model and generate the datapages package.      |
| `datapages watch`   | Start the live-reloading development server.                 |
| `datapages lint`    | Validate the app model without generating code.              |
| `datapages version` | Print CLI version information.                               |

## Configuration

Nothing about the build is configured. Every setting is read from the code that already states it.

Static file serving is turned on by an `embed.FS` whose doc comment names the URL path it is served at, the way a page names its route. The comment gives the prefix, the `//go:embed` directive gives the directory:

```go
// app/assets.go

// StaticFS is /static/
//go:embed static/*
var StaticFS embed.FS
```

The URL path in the comment must start and end with `/` and cannot be `/`. The `//go:embed` directive must name exactly one directory inside the app package.

The `browsable` argument of `datapages.WithAssets` lists a directory that has no `index.html`. Pass `browsable=false` in production to avoid exposing every embedded file.

Datapages adds no `Cache-Control` or `ETag` header unless `datapages.WithAssetsCache` is configured.

The app package, the session data type, the metrics mode and the package to generate into are the type arguments of the `datapages.NewServer` call.

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

`App` names the app package. `Metrics` decides the Prometheus counters: `datapages.EnablePrometheus` generates the code that counts and requires `WithPrometheus` to serve it, `datapages.DisablePrometheus` generates no counters, has no Prometheus imports and rejects that option. Use `datapages init --prometheus=false` to scaffold a project whose entry point names it. `S` must name that app package's `datapagesgen`, which is where its code is generated: `app/datapagesgen` for `./app`, `app/frontend/datapagesgen` for `./app/frontend`.

One module may build any number of applications. Each app package gets its own model, its own generated package and its own entry point:

```
app/frontend/                 cmd/frontend/
app/frontend/datapagesgen/
app/admindashboard/           cmd/admindashboard/
app/admindashboard/datapagesgen/
```

Events are the one thing the applications of a module share: two of them given one broker publish into one namespace. No two of them may claim the same subject, which `datapages gen` and `datapages lint` check over the whole module. To let two applications receive each other's events, declare the event once in a package both import and use that type in both, instead of declaring it twice.

`datapages gen` generates every one of them. `datapages watch` runs one, so a module that builds more than one needs `--app` to say which:

```sh
datapages watch --app frontend
```

A module with no `NewServer` call yet is generated into `app/datapagesgen` from `./app` and gets a `cmd/server/main.go` written for it.

What is left is the tooling, which `datapages.yaml` or `datapages.yml` in the module root carries. If both files exist, the CLI treats that as an error.

```yaml
cmd: cmd/server
watch:
  exclude:
    - ".git/**" # git internals
    - ".*"      # hidden files/directories
    - "*~"      # editor backup files
```

These top-level keys are supported:

- `cmd`: where `datapages gen` writes the first `cmd/server/main.go`,
- and which command `datapages watch` builds while no `NewServer` call is written in a `main` package yet. Default: `cmd/server`. Once such a call exists, the command it is written in is the entry point and this key is unused, which is why a module building several applications does not set it. Must be a relative path inside the module: an absolute path or a `..` segment is rejected.
- `watch`: optional development server settings (app host, proxy timeout, debounce, TLS, compiler flags, logging, custom watchers, etc.)

## Per-tab state

A handler that declares `datapages.State[T]` is given a value that belongs to the browser tab the request came from. Two tabs of the same page hold two values, and handlers of one tab are serialized against each other, so a handler reads and writes its fields without locking:

```go
// TabFilters is the per-tab state of PageIndex.
type TabFilters struct {
	Search string
	Sort   string
}

// POSTFilter is /filter
func (p PageIndex) POSTFilter(
	r *http.Request,
	sse datapages.SSE,
	state datapages.State[TabFilters],
	signals datapages.Signals[struct {
		Search string `json:"search"`
	}],
) error {
	state.Values.Search = signals.Values.Search
	// Read out what is needed; the pointer must not outlive this handler.
	return sse.PatchElement(results(p.App.Search(state.Values.Search)))
}
```

`state.Values` must not outlive the handler that received it. The per-tab mutex serializes handlers, not a goroutine one of them started, so a goroutine that keeps the pointer races with the tab's later handlers. Storing it in the application keeps the state alive after the tab is gone. Copy the fields out instead.

The state lives in server memory for exactly as long as the tab holds its SSE stream: a stream that drops takes it, and a reconnect starts from a zeroed value. The page load mints a random tab id and returns it in the `Datapages-Instance` header. The id is not written to a cookie or browser storage. The client shim stores only a reload marker in `sessionStorage`. A page that takes state gets that stream whether or not it declares `StreamOpen`, `StreamClose` or an `OnXXX` handler. The default instance limit needs no configuration. `WithStateConfig` sets a different limit.

See [`datapages.State[T]`](SPECIFICATION.md#parameter-datapagesstatet) for the declaration rules, the configuration, and what a client is told when its state is gone.

## Specification

See [SPECIFICATION.md](SPECIFICATION.md) for the full source package specification, including handler signatures, parameters, return values, events, sessions, and modules.

See [FAQ.md](FAQ.md) for frequently asked questions.

## Modules

Datapages ships pluggable modules with swappable implementations:

- [`Manager[Data]`](modules/sessions/sessions.go)
  - [`natskv`](https://pkg.go.dev/github.com/romshark/datapages/modules/sessions/natskv) - NATS KV store with AES-128-GCM encrypted cookies
  - [`inmem`](https://pkg.go.dev/github.com/romshark/datapages/modules/sessions/inmem) - In-memory sessions (lost on restart; single-instance only)
- [`Broker`](modules/messaging/messaging.go)
  - [`natscore`](https://pkg.go.dev/github.com/romshark/datapages/modules/messaging/natscore) - Core NATS backed message broker
  - [`inmem`](https://pkg.go.dev/github.com/romshark/datapages/modules/messaging/inmem) - In-memory fan-out message broker (single-instance only)
- [`TokenWriter`, `TokenValidator`](modules/csrf/csrf.go)
  - [`Tokens`](modules/csrf/tokens.go) - the built-in default: HKDF-SHA256 over the session token, BREACH-resistant masking, nothing to configure
- [`TokenGenerator`](modules/sessions/sessions.go)

## Motivation

The reason I built Datapages is that the combination of [Datastar](https://data-star.dev) + [Go](https://go.dev) + [Templ](https://templ.guide) is my preferred way of writing server-centric web applications. But in every project I used this tech stack for I kept repeating the same code patterns and solving the same problems over and over again. I realized many developers are repeating the same patterns too and struggle with the common pitfalls:

- How to handle SSE streams correctly?
- How to use NATS effectively?
- How to approach security and authentication?
- How to configure a convenient hot-reload for development?
- How to keep the code maintainable over time, especially when you add more developers and/or AI assistants?
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

- **Already write your backend in Go** and want to build your web frontend in the same language and toolchain.
- **Are building a server-rendered application**, where the server owns the data; not a local-first offline-capable SPA.
- **Already use [Datastar](https://data-star.dev)** and want a Go framework to help you ship faster with less code while preserving maintainability.
- **Already use [Templ](https://templ.guide)** and want a full framework built around it.
- **Use [HTMX](https://htmx.org/), [idiomorph](https://htmx.org/extensions/idiomorph/) and [Alpine.js](https://alpinejs.dev/)**, and instead want a single cohesive stack with a smaller bundle size and less spaghetti-code.
- **Don't want to maintain a separate REST/GraphQL API** just to feed your frontend.
- **Want to deploy as a single, statically compiled binary** that makes the most of your hardware.
- **Want to develop hybrid desktop apps** in Go and HTML5 (see [Calculator example](https://github.com/romshark/datapages/tree/main/example/calculator))

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and commands, and [AGENTS.md](AGENTS.md) for code style, testing conventions, commit message format and project structure.
