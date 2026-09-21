# Getting Started

Datapages turns a Go application model into an HTTP server, typed URL and action helpers, event dispatchers, and browser integration. This guide follows the path from a new project to the features most applications add next.

Declaration rules, valid handler signatures, lifecycle details, and error behavior are defined in [SPECIFICATION.md](SPECIFICATION.md). This guide explains when to use each feature and shows how the pieces fit together.

## Requirements

- The latest [Go toolchain](https://go.dev/dl/).
- [templ](https://templ.guide) `v0.3.1020`, which matches the scaffold and its CI workflow.
- [Docker](https://www.docker.com/) for the scaffold's NATS service. A single-process application can use the in-memory modules instead.

Install the CLI and templ:

```sh
go install github.com/romshark/datapages/cmd/datapages@latest
go install github.com/a-h/templ/cmd/templ@v0.3.1020
```

## Create and run a project

Run the initializer:

```sh
datapages init
```

The command asks for missing project settings, creates the scaffold, resolves dependencies, and generates the initial server. Accept its prompt to start the application, or run:

```sh
make dev
```

`make dev` starts the scaffold's NATS container and `datapages watch`. Open the URL printed by the watcher.

Use `datapages init --prometheus=false` when the first version of the project does not need metrics. Use `--no-ai-skills` to omit agent instructions.

## Read the scaffold

The initial project has four main parts:

```text
.
├── app/                 application model and templ components
│   ├── app.go
│   ├── app.templ
│   └── datapagesgen/    generated server and typed helpers
├── cmd/
│   └── server/
│       └── main.go      dependencies, server options, and process startup
└── datapages.yaml       development tooling configuration
```

Edit `app/` and `cmd/server/`. Do not edit `datapagesgen/` or files with a `DO NOT EDIT` header. Regeneration replaces them.

`app/app.go` starts with `App`, `PageIndex`, error pages, and error recovery. `app/app.templ` renders the first page. `cmd/server/main.go` connects the message broker, constructs `App`, and calls `datapages.NewServer`.

## Use the development loop

Keep `datapages watch` running while editing. It regenerates code, rebuilds the server, and reports failures in the browser preview.

Run the individual tools when diagnosing a build or checking CI locally:

```sh
templ generate
datapages gen
datapages lint
go build ./...
```

Run `templ generate` after changing a `.templ` file. Run `datapages gen` after changing the application model. `datapages lint` applies the generator checks without writing generated code.

Generator failures name the declaration and usually include a suggested fix. Fix the application package and regenerate; do not patch the generated output.

## Understand the application model

The application package is the model. Datapages reads its Go types, methods, and route or subject comments. There is no separate route table or handler configuration file.

`App` holds long-lived dependencies such as repositories and API clients. Pages render documents. Actions handle browser commands. Events update open pages when data changes elsewhere. Stream hooks manage resources attached to an open page stream.

Start with the generated `PageIndex`. Add one feature at a time and let `datapages gen` report declarations it cannot use. The [source package specification](SPECIFICATION.md#source-package) defines all recognized types, methods, comments, parameters, and return values.

As the page model grows, use these reference sections:

- [Abstract page types](SPECIFICATION.md#abstract-page-types) for shared page behavior.
- [`body`](SPECIFICATION.md#return-value-body-datapagescomponent) and [`head`](SPECIFICATION.md#return-value-head-datapageshead) for rendered output.
- [`redirect`](SPECIFICATION.md#return-value-redirect-datapagesredirect) for navigation.
- [Background streaming](SPECIFICATION.md#get-return-value-enablebackgroundstreaming-datapagesenablebackgroundstreaming) and [refresh control](SPECIFICATION.md#get-return-value-disablerefreshafterhidden-datapagesdisablerefreshafterhidden)
  for tab-visibility behavior.

## Add a page action

Start with a fragment the action can render. Replace the generated page body in `app/app.templ`:

```templ
templ pageIndex() {
	<main>
		@status("Ready")
	</main>
}

templ status(text string) {
	<p id="status">{ text }</p>
}
```

Run `templ generate` so the component is available to Go. Then add the action to `app/app.go`:

```go
// POSTUpdateStatus is /status
func (PageIndex) POSTUpdateStatus(
	r *http.Request,
	sse datapages.SSE,
) error {
	return sse.PatchElement(status("Updated"))
}
```

Run `datapages gen`. The generated `action` package now has a builder for the new route. Import it from your module in `app/app.templ`, then add the button inside `main`:

```templ
<button
	type="button"
	data-on:click={ action.PageIndex.UpdateStatus.POST() }
>
	Update status
</button>
```

Run `templ generate`, or let the watcher regenerate it. The button now calls the generated route, and the handler renders the updated fragment.

This pattern keeps routing in generated Go and rendering in templ. The [page specification](SPECIFICATION.md#pages) defines action signatures and the [`SSE` parameter](SPECIFICATION.md#parameter-sse-datapagessse) defines the available browser updates.

## Read request data

Choose input by where the value belongs:

- Use `datapages.Path` for route values.
- Use `datapages.Query` for URL state that should be bookmarkable or shareable.
- Use `datapages.Signals` for Datastar state sent with an action.
- Use `*http.Request` for headers, context, and other HTTP data.

The specification defines the supported field types and tags for [`Path`](SPECIFICATION.md#parameter-datapagespathstruct-), [`Query`](SPECIFICATION.md#parameter-datapagesquerystruct-), and [`Signals`](SPECIFICATION.md#parameter-datapagessignalsstruct-). It also defines query-to-signal reflection.

## Update more than the acting tab

An action with `datapages.SSE` updates the request's page. Use an event when a change must reach other open pages or other server processes.

A common flow is:

1. Validate the command.
2. Change authoritative application data.
3. Dispatch an event.
4. Re-read that data in each `OnXXX` handler.
5. Render a complete current fragment.

Treat events as notifications, not as the source of truth. This keeps a page correct after reconnecting or missing an update.

The [`Dispatcher`](SPECIFICATION.md#parameter-datapagesdispatchereventxxx) section defines event declarations, handlers, and subject fields. Read [signal-scoped subject fields](SPECIFICATION.md#signal-scoped-subject-fields) when a subscription depends on browser state, and read [event delivery](SPECIFICATION.md#event-delivery) before relying on an event for UI consistency. Shared event types are covered under [events outside the application package](SPECIFICATION.md#events-declared-outside-the-application-package).

## Keep temporary state per tab

Use `datapages.State[T]` for ephemeral server-side values owned by one browser tab. Typical examples are an unfinished wizard, a temporary filter, or a selection needed by an event handler.

Keep durable domain data in a repository or another application dependency. Do not use per-tab state as a session store.

Use `StreamOpen` to initialize stream resources or state and `StreamClose` to release them. The [`State[T]` specification](SPECIFICATION.md#parameter-datapagesstatet) defines declaration rules, synchronization, lifecycle, limits, security, and multi-server routing. The [page specification](SPECIFICATION.md#pages) defines stream-hook signatures.

## Add sessions and authorization

Sessions identify a visitor and expose application-defined session data. Add a session manager in `cmd/server/main.go`, then accept the application's session type in the handlers that need identity.

Keep authorization in the application. A valid session answers who sent the request, not whether that user may perform an operation.

The [`Session` parameter](SPECIFICATION.md#parameter-session-datapagessessiondata), [`NewSession`](SPECIFICATION.md#return-value-newsession-datapagesnewsessiondata), and [`CloseSession`](SPECIFICATION.md#return-value-closesession-datapagesclosesession) sections define session behavior. Read [Plain Forms and CSRF](SPECIFICATION.md#plain-forms-and-csrf) before adding authenticated forms.

## Handle failures

Return ordinary Go errors from handlers. Use Datapages error sentinels when the client should receive a specific HTTP status. `App.RecoverError` can render feedback for failed Datastar requests; page error handlers cover page loads.

See the [`App`](SPECIFICATION.md#app) and [`error` return](SPECIFICATION.md#return-value-error-or-err-error) sections for panic, logging, response, and recovery behavior.

## Write templates with generated helpers

Use the generated `href` package for links and the generated `action` package for Datastar actions:

```templ
<a href={ href.PagePost(post.Slug) }>{ post.Title }</a>
<button data-on:click={ action.PageIndex.UpdateStatus.POST() }>
	Update status
</button>
```

The helpers keep templates synchronized with the Go model. `datapages lint` checks their use in `.templ` files. The [linting specification](SPECIFICATION.md#linting) defines the diagnostics, allowed URL forms, and suppression directive.

Keep application decisions in Go. Use templates for markup and Datastar attributes for browser interaction.

## Configure the server

`cmd/server/main.go` is ordinary Go. Construct repositories and clients there, put long-lived dependencies on `App`, configure the broker and session manager, then pass server options to `datapages.NewServer`.

Common options configure logging, middleware, HTTP timeouts, sessions, static assets, request limits, graceful shutdown, metrics, per-tab state, CSP nonces, and the Datastar bundle. Use the [`ServerOption` documentation](https://pkg.go.dev/github.com/romshark/datapages#ServerOption) for the option API. Read the [Content Security Policy](SPECIFICATION.md#content-security-policy) section before setting a CSP.

The generated server implements `http.Handler`. Standard Go middleware and HTTP tooling can serve it.

## Choose modules for the deployment

The scaffold uses NATS for messaging and sessions. Datapages also provides in-memory implementations for a single process.

| Need | Single process | Multiple processes |
| ---- | -------------- | ------------------ |
| Events | `modules/messaging/inmem` | `modules/messaging/natscore` |
| Sessions | `modules/sessions/inmem` | `modules/sessions/natskv` |

The in-memory session manager is suitable for development. A restart removes its sessions. Choose a durable session manager before production.

If the deployment uses per-tab state, follow the multi-server guidance in the [`State[T]` specification](SPECIFICATION.md#parameter-datapagesstatet) when running more than one application server.

## Serve static assets

The scaffold contains a commented `embed.FS` example. Enable it in the application package, pass it with `datapages.WithAssets`, and use the generated `assets` or `href` helper when writing a URL.

Use `datapages.WithAssetsCache` to set production cache behavior. The [dev-mode specification](SPECIFICATION.md#dev-mode) defines development loading and cache behavior. The [`WithAssets`](https://pkg.go.dev/github.com/romshark/datapages#WithAssets) documentation defines the filesystem options.

## Build multiple applications

One Go module can contain several application packages and entry points. Each `datapages.NewServer` call connects an app package to its generated package. `datapages gen` processes all of them; select one for development with:

```sh
datapages watch --app frontend
```

Put an event type in a shared package when applications exchange that event. The specification for [events outside the application package](SPECIFICATION.md#events-declared-outside-the-application-package) defines discovery and subject ownership.

The [`example/counter`](example/counter/) module shows two applications with separate entry points.

## Configure development tooling

`datapages.yaml` or `datapages.yml` configures generation and watch mode. The scaffold starts with:

```yaml
cmd: cmd/server
watch:
  exclude:
    - ".git/**"
    - ".*"
    - "*~"
```

Use `cmd` until the project has a `datapages.NewServer` call in a main package. Use `watch` for proxy settings, debounce, TLS, compiler flags, logging, and custom file watchers. Run `datapages watch --help` for command-line overrides.

## Use the generated agent instructions

`datapages init` writes `AGENTS.md` and tool-specific pointers, plus Datapages skills under `.agents/skills` and `.claude/skills`. The instructions describe the generated project and give coding agents task-specific guidance.

Run `datapages init -n` in an existing project to update them from the installed CLI. The command backs up a changed file before replacing it. Edit both skill directories when the project uses agents that read both locations.

## Test through HTTP

The generated server implements `http.Handler`. Use `httptest` to exercise page loads, actions, and streams at the boundary clients use. Keep domain logic in separate Go types and test it without HTTP.

The repository's [acceptance-test guide](internal/acceptance/README.md) shows how to construct generated servers and assert HTML and SSE responses. A successful `go build ./...` checks compilation, not request behavior.

## Continue

- [SPECIFICATION.md](SPECIFICATION.md): exact source-model and runtime rules.
- [example/](example/): complete applications from a counter to classifieds.
- [SECURITY.md](SECURITY.md): application and deployment responsibilities.
- [FAQ.md](FAQ.md): design decisions and common questions.
