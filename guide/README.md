# Introduction

Datapages is a webdev meta-framework and toolchain for [Go](https://go.dev/), [Templ](https://templ.guide), and [Datastar](https://data-star.dev), designed for building modern web applications and hybrid desktop apps with a thin-client architecture.

A Datapages application is a Go package that defines its pages, actions, and events using Go types and methods, and renders HTML using Templ components. The `datapages` CLI tool lints the code, enforces rules and generates the HTTP server and all the necessary wiring automatically, allowing developers and AI coding agents to focus on application logic rather than infrastructure and boilerplate. Datapages also includes `datapages watch`, which provides a development environment with hot reload and immediate feedback on code changes.

Datapages follows a **server-centric, thin-client architecture** with a clear separation of responsibilities:

- **The server** owns application logic, data, and state; handles actions, enforces invariants, dispatches events, and renders HTML using Templ components.
- **The client** uses Datastar as a lightweight presentation and interaction layer to send user input to server-side actions, handle local interactivity and reactivity, provide immediate visual feedback, and [morph](https://data-star.dev/guide/getting_started/#patching-elements) server-rendered HTML into the DOM in real-time using [server-sent events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events/Using_server-sent_events).

This architecture offers several advantages:

- **Less JavaScript:** Dramatically reduces the amount of JavaScript required to build fast, interactive, real-time, and collaborative applications.
- **Smaller bundles:** Minimizes client-side dependencies and JavaScript bundle sizes, improving loading times and reducing bandwidth consumption.
- **Better performance and resource efficiency:** Leverages Go's runtime efficiency, particularly for server-side rendering (SSR), for faster SEO, better [TTI](https://developer.mozilla.org/en-US/docs/Glossary/Time_to_interactive), better [FCP](https://developer.mozilla.org/en-US/docs/Glossary/First_contentful_paint) and other use cases. Generally speaking, it's almost always faster to render HTML on the server and displaying it in the browser, than fetching JSON from the server and using JavaScript to interpret this data and manipulate the DOM.
- **Improved maintainability:** Less boilerplate, less duplicated logic, strong static typing, and built-in linting help keep code simple and catch errors early.
- **Developer & AI productivity:** Automatically generates HTTP infrastructure and wiring, allowing developers and AI coding agents to focus on business logic.

The counter in [`example/counter/app/simple`](../example/counter/app/simple/) shows each part:

```go title="app/simple/app.go"
package simple

import (
	"net/http"
	"sync/atomic"

	"github.com/romshark/datapages"
)

// EventCounterUpdated is "simple.counter.updated"
type EventCounterUpdated struct{}

type App struct{ counter atomic.Int32 }

func (*App) Head(_ *http.Request) datapages.Head { return head() }

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return pageCounter(p.App.counter.Load()), nil
}

// POSTAdd is /add/{$}
func (p PageIndex) POSTAdd(
	r *http.Request, counterUpdated datapages.Dispatcher[EventCounterUpdated],
	query datapages.Query[struct {
		Delta int32 `query:"delta"`
	}],
) error {
	p.App.counter.Add(query.Values.Delta)
	return counterUpdated.Dispatch(EventCounterUpdated{})
}

func (p PageIndex) OnCounterUpdated(
	event EventCounterUpdated, sse datapages.SSE,
) error {
	return sse.PatchElement(pageCounter(p.App.counter.Load()))
}
```

```templ title="app/simple/app.templ"
templ pageCounter(value int32) {
	<div id="page-counter">
		<button data-on:click={ action.PageIndex.Add.POST(action.PageIndex.Add.POSTQuery(-1)) }>-</button>
		<span id="value">{ fmt.Sprint(value) }</span>
		<button data-on:click={ action.PageIndex.Add.POST(action.PageIndex.Add.POSTQuery(1)) }>+</button>
	</div>
}
```

- `PageIndex` is a page. The first line of its doc comment declares its route, `/`. `GET` renders the page with the templ component `pageCounter`. `App.Head` returns the `<head>` tags that every page gets.
- `POSTAdd` is an action. The `-` and `+` buttons of `pageCounter` call it through Datastar with a `delta` query parameter, as `@post('/add/?delta=1')`. It adds `delta` to the counter and dispatches `EventCounterUpdated`.
- `OnCounterUpdated` handles the event in every tab that has the page open. It renders the counter again and sends it over the SSE stream of the tab. Datastar morphs it into the element with the same ID.

`datapages gen` writes route handlers, page streams and typed template helpers below the app package in `datapagesgen`. The `href` package builds links, and the `action` package builds Datastar actions.

```mermaid
flowchart LR
    app["app package<br/>pages, actions,<br/>events, templates"]
    gen(["datapages gen"])
    generated["datapagesgen<br/>handlers, streams,<br/>links and actions"]
    server["HTTP server"]
    app --> gen --> generated --> server
    generated -. used by templates .-> app
    app --> server
```

The in-memory message broker delivers events within 1 server process. Use NATS across processes.

## When to use Datapages

Datapages fits an application whose server owns its data and rules. Use it when:

- the application is written mostly in Go and Templ;
- the browser sends commands instead of changing authoritative data itself;
- changes must reach other open tabs or server processes;
- the browser does not need a separate REST or GraphQL API; or
- the same app package runs as a web server and a desktop app, as in [`example/calculator`](../example/calculator/).

## When not to use Datapages

Use a client-side application when it must run independently of a server or deploy as static files to a CDN. For a static site, use Templ directly. This [documentation site](../internal/tools/render-pages/) does that. Datastar and Web Components can add client-side interaction without Datapages.

## How these docs are organized

- Start with [Installation](installation.md), then build the application in the [tutorial](tutorial/README.md).
- [Application model](application-model.md) explains the source and generated code. [Request lifecycle](request-lifecycle.md) follows a page load, stream, action and event through the server.
- Use the task guides for implementation and the [specification](../SPECIFICATION.md), [CLI](cli.md), [configuration](configuration.md) and [Go API](https://pkg.go.dev/github.com/romshark/datapages) for reference.

## Get help

- Ask questions in the #datapages channel on [Discord](https://discord.com/channels/1296224603642925098/1464634812135505940) or on the [Gophers Slack](https://gophers.slack.com/archives/C0AKMFC1M2A).
- Report bugs and propose features in [GitHub issues](https://github.com/romshark/datapages/issues). Include the output of `datapages version --full` and `go version` in a bug report.
- Report vulnerabilities privately as [SECURITY.md](../SECURITY.md) describes. Do not use a public issue.
