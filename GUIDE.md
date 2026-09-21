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

## Add offline page caching

The offline module installs a service worker. Handlers can cache page snapshots that the worker serves when the network is unavailable. Start by declaring the fallback for a URL that has no snapshot:

```go
// PageOffline is /offline
type PageOffline struct{ App *App }

func (PageOffline) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return pageOffline(), nil
}
```

`PageOffline` is shared by every visitor and renders with a zero session. Keep it independent of signed-in state.

Configure the worker in the application package. Precache every asset that a snapshot needs before its first online load, including a self-hosted Datastar bundle:

```go
const offlineWorkerVersion = 1

func OfflineConfig() offline.Config {
	return offline.Config{
		WorkerVersion: offlineWorkerVersion,
		Assets: []string{
			assets.Path("style.css"),
			assets.Path("datastar.js"),
		},
	}
}
```

Declaring `PageOffline` generates the option that supplies its route. Add these options to `datapages.NewServer`:

```go
datapages.WithDatastarJS(assets.Path("datastar.js")),
datapagesgen.WithOffline(app.OfflineConfig()),
```

The worker runs on HTTPS and localhost. On other plain HTTP origins the offline API does nothing.

Add `pageCache datapages.PageCacheWriter` to a `GET` or action. `Set` stores a body for offline use; the URL must come from the generated `href` package:

```go
func (p PageItem) GET(
	r *http.Request,
	pageCache datapages.PageCacheWriter,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (body datapages.Component, err error) {
	item, err := p.App.Item(r.Context(), path.Values.ID)
	if err != nil {
		return nil, err
	}
	if pageCache.Version() != item.Revision {
		pageCache.Set(href.PageItem(item.ID), itemOffline(item), item.Revision)
	}
	return itemPage(item), nil
}
```

`Version()` is the cached version of the current request URL, or zero when that URL is absent. An action can cache another URL, but cannot read that URL's current version. `Clear` removes one entry; `ClearAll` removes all page entries. Writes take effect together only after the handler returns without error.

Apply these rules when choosing what to cache:

- Pass a page body, not a complete HTML document. Datapages supplies the document shell.
- Include every value that changes the snapshot in its version. Use `!=` for hashes and other unordered version keys.
- Cache a separate body when offline interactions cannot work. A snapshot does not need to match the live page.
- Treat the cache as origin-readable browser storage that outlives a session. Do not store secrets or data that must disappear at sign-out. Call `ClearAll` during sign-in and sign-out, then repopulate entries from later handlers.
- Update or clear affected URLs when an action changes their data.
- Increment `WorkerVersion` after a Datapages upgrade or a change to `Assets`, `ExcludePaths`, `CrossOriginDestinations`, `OfflineClass`, or `PageOffline`. A file update at an unchanged asset URL refreshes behind the cached copy and does not require a new worker version.

Same-origin assets are cached on their first online load unless excluded. `Config.Assets` makes required files available immediately after worker installation. `Config.CrossOriginDestinations` controls which cross-origin asset types may be cached. Use the default `is-offline` class on `<html>` to disable controls that require the network.

Register response compression before `WithOffline`; the offline middleware must edit the unencoded HTML before it is compressed. With a nonce-based CSP, `WithCSPNonce` must return the same nonce for every call with one request. The generated offline option applies that nonce to its inline scripts.

The [`pageCache` specification](SPECIFICATION.md#parameter-pagecache-datapagespagecachewriter) defines valid handlers and write delivery. [Service Worker](SPECIFICATION.md#service-worker) defines installation, caching, CSP, and response behavior. [`example/offline-cache`](example/offline-cache/) is a complete session-aware application.

## Show a cached shim while a page loads

Use `SetShim` when a live page is slow. The worker serves the cached body immediately, fetches the live page in parallel, then uses Datastar to replace the document head and body:

```go
const shimVersion = 1

func (p PageIndex) GET(
	r *http.Request,
	pageCache datapages.PageCacheWriter,
) (body datapages.Component, err error) {
	rows, err := p.App.Rows(r.Context())
	if err != nil {
		return nil, err
	}
	if pageCache.Version() != shimVersion {
		pageCache.SetShim(href.PageIndex(), indexShim(len(rows)), shimVersion)
	}
	return indexPage(rows), nil
}
```

The first visit waits for the live response and stores the shim. Later visits can display it before the live response arrives. Increment the shim's page-cache version when its markup or data changes; this version is independent of `offline.Config.WorkerVersion`.

A shim can appear online or remain visible after a failed fetch. Use neutral placeholder text, not an offline claim. It renders without a session or CSRF script and must contain no Datastar attributes. Put actions and visitor-specific content in the live body.

An application with `PageOffline` uses the same `datapagesgen.WithOffline` setup as snapshots. When an application needs shims but no custom offline fallback, install the module directly:

```go
offline.WithServiceWorker("", offline.Config{
	WorkerVersion: 1,
})
```

[`example/fast-shim`](example/fast-shim/) compares shimmed pages with uncached pages under the same artificial delay.

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
