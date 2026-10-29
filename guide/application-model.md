# Application model

Datapages generates an application's HTTP server from 1 Go package. The CLI finds this app package through `datapages.NewServer`, reads its declarations and writes `datapagesgen` below it.

## The app package

The app package holds the `App` type, pages, handlers, events and templ components. Together they form the application model. There is no route table. A page type declares its route on the first line of its doc comment.

The scaffold that `datapages init` writes keeps the app package in `app/` and the entry point in `cmd/server/`:

```text
.
|-- app/
|   |-- app.go            the application model
|   |-- app.templ         templ components
|   |-- app_templ.go      written by templ generate
|   `-- datapagesgen/     written by datapages gen
|-- cmd/
|   `-- server/
|       `-- main.go       dependencies, server options, datapages.NewServer
|-- datapages.yaml        settings of datapages watch
`-- go.mod
```

### How the CLI finds it

`datapages gen`, `datapages lint` and `datapages watch` find the app package through the `datapages.NewServer` call of the entry point:

```go title="cmd/server/main.go"
s, err := datapages.NewServer[
	app.App,                     // the application type, which names the app package
	datapages.DisableSessions,   // the session data type, or DisableSessions
	datapages.DisablePrometheus, // EnablePrometheus or DisablePrometheus
	datapagesgen.Server,         // the Server type of app/datapagesgen
](a, broker, opts...)
```

A module without any `NewServer` call, such as a project whose entry point is not written yet, is generated for the package in `./app`. That run also writes `cmd/server/main.go` with a `NewServer` call when the directory holds no Go files. The `cmd` setting of [`datapages.yaml`](configuration.md) names another directory. From then on the CLI reads the call in `main.go`. [Project discovery](cli.md#project-discovery) lists the exact rules of the scan. A module can hold several app packages, each with its own entry point. See [Multiple applications](multiple-applications.md).

## What Datapages reads

Datapages reads the names and types of the package-level declarations and the first line of their doc comments:

```go title="app/app.go"
package app

import (
	"embed"
	"net/http"

	"github.com/romshark/datapages"
)

// StaticFS is /static/
//
//go:embed static
var StaticFS embed.FS

type App struct{ posts *Posts }

// EventPostPublished is "post.published"
type EventPostPublished struct {
	Slug string `json:"slug"`
}

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return pageIndex(p.App.posts.Published()), nil
}

func (p PageIndex) OnPostPublished(
	event EventPostPublished, sse datapages.SSE,
) error {
	return sse.PatchElement(postList(p.App.posts.Published()))
}

// PagePost is /post/{slug}
//
// Shows 1 post with its comments.
type PagePost struct{ App *App }

func (p PagePost) GET(
	r *http.Request,
	path datapages.Path[struct {
		Slug string `path:"slug"`
	}],
) (body datapages.Component, err error) {
	post, ok := p.App.posts.Get(path.Values.Slug)
	if !ok {
		return nil, datapages.ErrNotFound
	}
	return pagePost(post), nil
}

// POSTPublish is /publish/{slug}
func (a *App) POSTPublish(
	r *http.Request,
	path datapages.Path[struct {
		Slug string `path:"slug"`
	}],
	published datapages.Dispatcher[EventPostPublished],
) error {
	if !a.posts.Publish(path.Values.Slug) {
		return datapages.ErrNotFound
	}
	return published.Dispatch(EventPostPublished{Slug: path.Values.Slug})
}
```

`Posts` is an ordinary type in another file of the package, and `pageIndex`, `postList` and `pagePost` are templ components in `app/app.templ`. Datapages ignores declarations it does not recognize. From the declarations above it generates:

| Declaration | Recognized by | Generated |
| --- | --- | --- |
| `App` | the name `App`, which is required | the value every handler reaches through `p.App` |
| `PageIndex` | the `Page` prefix, the field `App *App` and the route comment | the route `GET /` and `href.PageIndex()` |
| `PageIndex.OnPostPublished` | the `On` prefix and a parameter of type `EventPostPublished` | the page stream `GET /_$/{$}`, subscribed to `post.published` |
| `PagePost` | the same as `PageIndex` | the route `GET /post/{slug}/{$}` and `href.PagePost(slug)` |
| `(*App).POSTPublish` | the `POST` prefix and the route comment | the route `POST /publish/{slug}/{$}` and `action.App.Publish.POST(slug)` |
| `EventPostPublished` | the `Event` prefix and the quoted subject in its comment | the subject `post.published` and the dispatcher behind `datapages.Dispatcher[EventPostPublished]` |
| `StaticFS` | an `embed.FS` variable whose comment names a URL path | the file server at `/static/`, the `assets` package and `href.Asset` |

### Declarations

These package-level declarations have a meaning:

- `App`, a type that must exist. The handlers reach it through the `App *App` field of their page. Put long-lived dependencies, such as repositories and API clients, on it.
- Pages, struct types named `Page` followed by an uppercase letter. A page declares exactly 1 named field, `App *App`, and a `GET` method. It may also embed stream embeds. `PageIndex` is required and serves `/`. Its route also catches every path that no other route matches, which it answers with the 404 page.
- Special pages: `PageError404` and `PageError500` replace the default error pages, and `PageOffline` is the page the [offline worker](offline.md) shows for an uncached URL. Each declares its route like any other page.
- Stream embeds, structs whose name does not start with `Page` and that have the field `App *App`. A page embeds them to share event handlers, `StreamOpen` and `StreamClose` with other pages.
- Events, types named `Event` followed by an uppercase letter, with the comment `// EventXXX is "subject"`. An event type may also come from an imported package. See [Events](events.md).
- Per-tab state, an exported struct of the app package used as `T` in `datapages.State[T]`. See [Per-tab state](state.md).
- The session data type, the `Data` of `datapages.Session[Data]` or `datapages.NewSession[Data]` in a handler. Every handler uses the same `Data`. See [Sessions and auth](sessions.md).
- Assets, at most 1 `embed.FS` variable whose comment names a URL path, as `// StaticFS is /static/`. See [Static assets](assets.md).

A page that has an event handler, a stream hook or per-tab state gets an SSE stream at its route followed by `_$/`. [Request lifecycle](request-lifecycle.md) shows how a page uses it.

### Methods

The prefix of a method name decides its role:

| Method | Owner | Role |
| --- | --- | --- |
| `GET` | page | renders the page |
| `GETXXX` | page, `*App` | answers `GET` with a `datapages.File` |
| `POSTXXX`, `PUTXXX`, `PATCHXXX`, `DELETEXXX`, `QUERYXXX` | page, `*App` | an action called from the browser |
| `OnXXX` | page, stream embed | handles `EventXXX` in the open tabs of the page |
| `StreamOpen`, `StreamClose` | page, stream embed | run when the page stream opens and closes |
| `StreamConfig` | page | configures the compression of the page stream |
| `StreamConfigXXX` | page, `*App` | configures the SSE response of the action `XXX` |
| `Head` | `*App` | renders the `<head>` tags of every page |
| `RecoverError` | `*App` | renders the error of a failed Datastar request |

Declare page methods on a value receiver and `App` methods on `*App`. An unexported method is an ordinary method. An exported page method with any other name is an error. `App` may declare other exported methods whose names start with none of these prefixes, and Datapages ignores them.

### Routes

The first line of the doc comment is a [net/http ServeMux pattern](https://pkg.go.dev/net/http#hdr-Patterns-ServeMux), as in `// PagePost is /post/{slug}`. Put a description after an empty `//` line. Datapages registers the route with a trailing slash and `{$}`, as in `GET /post/{slug}/{$}`. The `PageIndex` route `/` and routes ending in `{name...}` keep their original form. The server appends a trailing slash to every other request path outside the asset prefix. `/post/hello` and `/post/hello/` therefore reach the same page.

The route of a page action starts with the route of its page. `POSTComment` on `PagePost` can be `/post/{slug}/comment` but not `/comment`. An action on `App` can have any route. Each route variable, such as `{slug}`, needs a field in the handler's `datapages.Path` parameter. See [Pages and routes](pages.md).

### Parameters and return values

The generator matches handler parameters and return values by type, not by name or position. A handler declares only the values it needs. This action takes them in an unusual order, and the generated code passes each by its type:

```go title="app/comment.go"
// POSTComment is /post/{slug}/comment
func (p PagePost) POSTComment(
	signals datapages.Signals[struct {
		Text string `json:"text"`
	}],
	sse datapages.SSE,
	path datapages.Path[struct {
		Slug string `path:"slug"`
	}],
	r *http.Request,
) error {
	post, ok := p.App.posts.AddComment(path.Values.Slug, signals.Values.Text)
	if !ok {
		return datapages.ErrNotFound
	}
	return sse.PatchElement(comments(post))
}
```

The only name with a meaning is `stateID string`, which a handler with per-tab state takes for the ID of its tab. A parameter of a type the handler kind does not accept is an error. The [specification](../SPECIFICATION.md#pages) lists the parameters and return values of every handler kind.

## Naming rules

`datapages gen` and `datapages lint` reject a declaration that claims a role but breaks its naming rule:

- After the prefix `Page`, `On`, `GET`, `POST`, `PUT`, `PATCH`, `DELETE` or `QUERY` comes an uppercase letter, then only letters and digits. `Page_Help` and `POSTsave` are rejected. A method whose name starts with a lowercase letter, as `postSave`, is an ordinary method.
- Every type whose name starts with `Page` is read as a page. `type PageSize int` is rejected with `page type must be a struct type`. Give such types other names.
- A type is an event only when its name is `Event` followed by an uppercase letter and letters and digits. `Event_Log` is an ordinary type.
- `OnXXX` repeats the name of its event after `Event`, as `OnPostPublished` for `EventPostPublished`, also for an event declared in another package.
- The comment line uses the word `is`. A route is unquoted, as `// PagePost is /post/{slug}`. An event subject is quoted, as `// EventPostPublished is "post.published"`.
- A page type is a struct type literal whose only named field is `App *App`.
- `App`, page, stream embed and event types have no type parameters.
- The app package has no build-constrained files, such as a file with a `//go:build` line or a name ending in `_windows.go`, except tests and files under `//go:build ignore`.

The parser also rejects routes that conflict, an action route outside its page's route, a route character that must be percent-encoded, such as `?` or `#`, and 2 event types with the same subject. [Linting](../SPECIFICATION.md#linting) lists the template checks that run in the same pass.

## What `datapages gen` writes

`datapages gen` rewrites everything below `datapagesgen/` on each run. The todolist example shows a typical result:

```text
example/todolist/app/datapagesgen/
|-- app_gen.go
|-- action/
|   `-- action_gen.go
|-- assets/
|   `-- assets_gen.go
`-- href/
    `-- href_gen.go
```

- `app_gen.go`, package `datapagesgen`: the `Server` type and its `Init` method, which `datapages.NewServer` calls. It registers the routes and holds the handler of every page, page stream and action, the dispatcher of every event, the subject constants and `MessageBrokerStreamSubjects`, and the store of per-tab state. An app with `PageOffline` also gets `WithOffline` here.
- `href/href_gen.go`, package `href`: 1 function per page that returns its URL, such as `href.PagePost(slug)`, and 1 per GET action. `PageError500` has none. A query struct of a page becomes a type such as `href.QueryPageIndex`. `href.External` marks an external URL. An app package with assets also gets `href.Asset`.
- `action/action_gen.go`, package `action`: 1 builder per action that returns the Datastar expression calling it, such as `action.App.Publish.POST(slug)`, which returns `@post('/publish/hello/')`. A GET action has none. The options of the expression, such as `action.WithContentType`, are here too.
- `assets/assets_gen.go`, package `assets`, only for an app package that declares assets: the URL prefix, the embedded directory, the directory dev mode reads from, and `assets.Path`.

A run also does the following:

- It parses and checks every app package of the module before it writes any of them. An app package with errors keeps the code an earlier run generated for it, and the others are generated. An app package that was never generated gets stub packages, which let editors resolve its imports while you fix the errors.
- It refuses to run when `go.mod` requires a newer Datapages than the CLI, and it raises the requirement to the CLI's version when the CLI is newer.
- It runs `go mod tidy`.
- It prints a notice when the [agent instructions](ai-agents.md) of the project came from another CLI version.

`datapages gen` does not run `templ generate`.

## What you write and what is generated

| File | Written by | Edit it |
| --- | --- | --- |
| `app/*.go` | you | yes |
| `app/*.templ` | you | yes |
| `app/*_templ.go` | `templ generate` | no, edit the `.templ` file |
| `app/datapagesgen/**` | `datapages gen`, on every run | no, change the app package and regenerate |
| `cmd/server/main.go` | `datapages init` or the first `datapages gen`, when missing | yes, it is yours after the first run |
| `datapages.yaml` | `datapages init` or `datapages watch`, when missing | yes |
| `.env`, `compose.yaml`, `Makefile`, `.github/workflows/ci.yml` | `datapages init`, when missing | yes |
| `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.agents/skills/`, `.claude/skills/` and the other agent files | `datapages init`, on every run | edits are replaced and kept in a `.bak` file, see [AI agents](ai-agents.md) |
| `go.mod`, `go.sum` | you, `go mod tidy` | yes |

Every generated Go file starts with `// Code generated by github.com/romshark/datapages; DO NOT EDIT.` The scan treats a `datapagesgen` directory holding Go files without that header as a package Datapages did not write, and refuses to generate into it.

## The build loop

Generate the server from the application model, then the templates, then build:

```sh
datapages gen    # parse the app package, write app/datapagesgen
templ generate   # compile app/*.templ into app/*_templ.go
go build ./...
```

The 2 generators depend on each other. `datapages gen` type-checks the app package, including the `*_templ.go` files, and refuses a package that does not compile. Running it first lets the templates call the `href` and `action` helpers of new declarations. 2 cases need another order:

- Go code calls a templ component that `templ generate` has not compiled yet. `datapages gen` then reports it as undefined, as `undefined: pageSearch`. Run `templ generate` first.
- A new template calls a helper of a new declaration, and Go code calls that template. Neither order works. Add the declaration and the template without the helper call, run both generators, then add the call and run `templ generate` again.

[Troubleshooting](troubleshooting.md#datapages-gen-reports-an-undefined-href-or-action-helper) shows the error and the recovery.

### `datapages watch`

`datapages watch` runs the build loop and serves the application through a development proxy on `localhost:7331`. It runs `datapages gen` at startup and after changes to the Go files of the app packages and of the packages that call `datapages.NewServer`. It also runs templ in watch mode, rebuilds and restarts the server, and reloads open tabs. Generator errors appear in the terminal and browser. A second proxy, based on [Toxiproxy](network.md), serves the application on a simulated slow or failing network. The server uses [dev mode](../SPECIFICATION.md#dev-mode), which reads assets from disk and applies `.templ` text changes without a rebuild.

Do not run `templ generate` while `datapages watch` runs. It deletes the temporary files that templ watch mode renders from, and the dev server serves blank pages until the next change to a `.templ` file. See [Troubleshooting](troubleshooting.md#the-dev-server-serves-blank-pages).

### `datapages lint`

`datapages lint` applies the checks of `datapages gen` and writes nothing. The checks include the `.templ` files of the app package and of the module packages it imports, for hardcoded URLs and for action calls in the wrong place. Run it in CI and in editors. See [Linting](../SPECIFICATION.md#linting) and [CLI](cli.md).

## Errors and fix suggestions

`datapages gen` and `datapages lint` print 1 entry per error with its position and message. When the CLI can derive a fix, a `fix:` line follows. This file has 3 mistakes. `PageAbout` has no route comment, `PageSearch.GET` takes a bare struct for its query, and `OnUpdate` does not name its event:

```go title="app/mistakes.go"
package app

import (
	"net/http"

	"github.com/romshark/datapages"
)

// EventCounterUpdated is "counter.updated"
type EventCounterUpdated struct{}

// The about page.
type PageAbout struct{ App *App }

func (PageAbout) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}

// PageSearch is /search
type PageSearch struct{ App *App }

func (PageSearch) GET(
	r *http.Request,
	query struct {
		Term string `query:"q"`
	},
) (body datapages.Component, err error) {
	return nil, nil
}

func (PageSearch) OnUpdate(event EventCounterUpdated, sse datapages.SSE) error {
	return nil
}
```

`datapages gen` prints:

```text
at mistakes.go:13:6: page is missing path comment: PageAbout
fix: Add `// PageAbout is /about/`

at mistakes.go:24:2: unsupported input parameter query struct{Term string "query:\"q\""} in PageSearch.GET
fix: Potential candidates: datapages.Path[...], datapages.Query[...], datapages.Signals[...]

at mistakes.go:31:19: event handler must be named after its event: PageSearch.OnUpdate handles EventCounterUpdated
fix: Rename PageSearch.OnUpdate to OnCounterUpdated

error: parsing app package: 3 error(s)
```

Positions are relative to the directory of the app package. For a parameter of an unsupported type, the `fix:` line lists the parameter types the value could be wrapped in that the handler has not taken yet. The command exits with status 1 and leaves the generated code as the last successful run wrote it. `NO_COLOR` turns the colors of a terminal off, and `FORCE_COLOR` turns them on in a pipe.

A package that does not compile reports the compiler errors and nothing else, since the model cannot be read without types:

```text
at app_templ.go:39:80: undefined: action.PageIndex

error: parsing app package: 1 error(s)
```

## See also

- [Source Package](../SPECIFICATION.md#source-package): every declaration, parameter and return value with its exact rules.
- [Request lifecycle](request-lifecycle.md): what the generated server does with a page load, a stream, an action and an event.
- [Pages and routes](pages.md) and [Actions](actions.md): writing pages and actions.
- [Multiple applications](multiple-applications.md): several app packages in 1 module.
- [CLI](cli.md): the flags of `datapages gen`, `datapages lint` and `datapages watch`.
- [Troubleshooting](troubleshooting.md): generator errors and their fixes.
