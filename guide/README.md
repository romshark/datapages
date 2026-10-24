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

This is an [example](../example/counter/app/simple/) of a simple server-centric collaborative real-time counter:

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

`datapages gen` writes the `datapagesgen` package below the app package. It contains the route handlers and page streams. Its `href` and `action` packages build links and Datastar actions for templates. A link to a page that does not exist then fails to compile. `datapages gen` and `datapages lint` reject declarations they cannot serve, such as a handler with an unsupported parameter type. They also report hardcoded URLs in templates.

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

Events are dispatched through a message broker. An in-memory message broker is more resource-efficient for single-instance deployment; for multi-instance NATS is required.

## When to use Datapages

Datapages fits applications whose server is the central authority and source of truth. Use it when:

- You want a fast modern frontend that is written almost entirely in Go.
- You want to use Datastar with Go in production with good tooling and focus on your business logic.
- You used to combine [HTMX](https://htmx.org/), [idiomorph](https://htmx.org/extensions/idiomorph/) and [Alpine.js](https://alpinejs.dev/) and want a overall better and more maintainable tech-stack.
- You want to avoid spending time designing and maintaining a REST, GraphQL, or similar APIs.
- You want a single easy to deploy binary executable.
- You're building a hybrid desktop GUI app with good performance, low resource footprint and a small distributable, while taking advantage of the vast ecosystem of Go and the web. See [`example/calculator`](../example/calculator/).

## When Not to Use Datapages 

If you’re building a CDN-hosted web application where the server isn’t the source of truth or the central authority, you should probably build a JavaScript single-page application instead.

If you need a static site generator, you should use Templ directly. This [documentation site](../internal/tools/render-pages/) is an example of that approach. Additionally, you can use Datastar and web components directly to add client-side interactivity to static sites.

## How these docs are organized

- Start with [Installation](installation.md), then build an application in the [tutorial](tutorial/README.md). [Upgrading](upgrading.md) moves an existing project to a newer release.
- [Application model](application-model.md) explains what Datapages reads and generates. [Request lifecycle](request-lifecycle.md) follows requests through the generated server.
- The task guides cover [pages](pages.md), [actions](actions.md), [templates](templates.md), [events](events.md), [per-tab state](state.md), [sessions](sessions.md), [assets](assets.md), [offline support](offline.md), [testing](testing.md), [deployment](deployment.md), [performance](performance.md), [security](security.md), [multiple applications](multiple-applications.md), [AI agents](ai-agents.md) and [troubleshooting](troubleshooting.md).
- The [specification](../SPECIFICATION.md) defines every declaration, parameter and return value. The [CLI reference](cli.md) lists commands and flags, and the [configuration reference](configuration.md) lists the `datapages.yaml` keys. The [Go API](https://pkg.go.dev/github.com/romshark/datapages) documents packages. The [changelog](../CHANGELOG.md) lists changes by release.
- [Examples](examples.md) describes the example applications in the repository. The [FAQ](../FAQ.md) explains design decisions.

## Get help

- Ask questions in the #datapages channel on [Discord](https://discord.com/channels/1296224603642925098/1464634812135505940) or on the [Gophers Slack](https://gophers.slack.com/archives/C0AKMFC1M2A).
- Report bugs and propose features in [GitHub issues](https://github.com/romshark/datapages/issues). Include the output of `datapages version --full` and `go version` in a bug report.
- Report vulnerabilities privately as [SECURITY.md](../SECURITY.md) describes. Do not use a public issue.
