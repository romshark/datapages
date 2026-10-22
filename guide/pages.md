# Pages and routes

A page is a struct type named `PageXXX`. Its doc comment declares the route, and its `GET` method renders an HTML document. Generated helpers build page URLs from typed path and query values.

## Declare a page

Declare a struct type whose name starts with `Page`, give it the field `App *App`, and write its route on the first line of its doc comment. A description goes below the route, after an empty `//` line:

```go title="app/app.go"
// PageAbout is /about
//
// Tells the story of the shop.
type PageAbout struct{ App *App }

func (PageAbout) GET(r *http.Request) (body datapages.Component, err error) {
	return aboutPage(), nil
}
```

`aboutPage` is a [templ](https://templ.guide) component in the same package:

```templ title="app/app.templ"
templ aboutPage() {
	<main>
		<h1>About us</h1>
		<p>We have sold lamps since 1998.</p>
	</main>
}
```

Run `templ generate` and `datapages gen`, or keep `datapages watch` running. The generated server then answers `GET /about/` with a document that contains the component, and the generated `href.PageAbout()` returns `/about/`.

`App *App` is the only named field a page may declare. The server creates a page value for each request with that field set. Put long-lived dependencies, such as a database or API client, on `App` and read them through `p.App`. Every application must declare `PageIndex` at `/`.

## Routes

A route is a [`net/http.ServeMux` pattern](https://pkg.go.dev/net/http#hdr-Patterns-ServeMux) without a method or host:

```go title="app/app.go"
// PageIndex is /
type PageIndex struct{ App *App }

// PageProduct is /products/{id}
type PageProduct struct{ App *App }

// PageDocs is /docs/{path...}
type PageDocs struct{ App *App }

// PageTerms is /terms/{$}
type PageTerms struct{ App *App }
```

- `{id}` matches 1 path segment. `/products/{id}` matches `/products/42/`.
- `{path...}` at the end of a route matches the rest of the path, slashes included. `/docs/{path...}` matches `/docs/guide/install`, and the handler reads `guide/install`.
- A route matches its own path and nothing below it. `/about` does not match `/about/team/`.
- Before it routes a request, the server appends a slash to a path that has none. `/about` and `/about/` reach the same page, and the generated `href` functions write the slash.
- A path that no route matches gets status 404 and the [404 page](#error-and-offline-pages). The route of `PageIndex` claims these paths, but its `GET` does not run for them.

The [Pages](../SPECIFICATION.md#pages) section of the specification lists the characters a route must percent-encode and the paths it cannot claim.

### Pages that serve only GET

A route that ends in `{$}` or in a `{name...}` wildcard declares a page that serves only `GET`. Such a page cannot have actions or a stream, because both are served at paths below the route. `datapages gen` rejects an action on it:

```text
at app.go:28:18: page route ending in {$} cannot have actions: PageTerms is "/terms/{$}" and has POSTAccept
fix: Remove {$} from the route of PageTerms
```

`{$}` does not change the URL. `/terms` and `/terms/{$}` both match `/terms/` and `/terms`, and nothing else. Write `{$}` for a page that must never get an action or a stream, and remove it when the page needs one.

## Path parameters

Read route variables with a `datapages.Path` parameter. Its struct has 1 exported field per variable, and the `path` tag names the variable:

```go title="app/app.go"
// PageProduct is /products/{id}
type PageProduct struct{ App *App }

func (p PageProduct) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID int64 `path:"id"`
	}],
) (body datapages.Component, err error) {
	product, ok := p.App.catalog.Product(path.Values.ID)
	if !ok {
		return nil, datapages.ErrNotFound
	}
	return productPage(product), nil
}
```

Every route variable needs a field, and every field needs a route variable. A field may be a `string`, a `bool`, an integer type, `float32`, `float64` or a type that implements `encoding.TextUnmarshaler`. The server parses the values before it calls `GET` and answers 400 to a value that does not parse, such as the `lamp` of `/products/lamp/`. Float fields reject `Inf` and `NaN`. `datapages.ErrNotFound` renders the [404 page](#error-and-offline-pages).

The generated builder takes the fields in route order with their Go types. `href.PageProduct(42)` returns `/products/42/`.

A path value arrives unescaped, which turns `%2F` into `/` and `%2E%2E` into `..`. Look the value up as a key, as `PageDocs` does with its wildcard, or open files through an `fs.FS` or an `os.Root`. Never join a path value into a file path:

```go title="app/app.go"
func (p PageDocs) GET(
	r *http.Request,
	path datapages.Path[struct {
		Path string `path:"path"`
	}],
) (body datapages.Component, err error) {
	doc, ok := p.App.docs.Lookup(path.Values.Path)
	if !ok {
		return nil, datapages.ErrNotFound
	}
	return docPage(doc), nil
}
```

## Query parameters

Read the query string with a `datapages.Query` parameter. The `query` tag names the key:

```go title="app/app.go"
// PageSearch is /search
type PageSearch struct{ App *App }

func (p PageSearch) GET(
	r *http.Request,
	query datapages.Query[struct {
		Term    string `query:"q"`
		Page    int    `query:"page"`
		InStock bool   `query:"stock"`
	}],
) (body datapages.Component, err error) {
	page := max(query.Values.Page, 1)
	results := p.App.catalog.Search(query.Values.Term, query.Values.InStock, page)
	return searchPage(results), nil
}
```

Query fields take the same types as path fields. A key that the URL lacks leaves its field at the zero value, which makes the zero value the default. Apply another default in the handler, as `max(query.Values.Page, 1)` does. A value that does not parse, such as `?stock=yes`, gets status 400. Of a repeated key, the first value counts. Each tag must be unique in its struct and may contain only ASCII letters, digits, `-`, `.`, `_` and `~`.

The generated builder takes the query as a struct named `href.Query` plus the page type, and leaves the fields with zero values out of the URL:

```go
href.PageSearch(href.QueryPageSearch{Term: "lamp", InStock: true}) // "/search/?q=lamp&stock=true"
```

### Keep the URL in sync with signals

A `reflectsignal` tag binds a query field to a [Datastar signal](templates.md#datastar-attributes), which keeps client state such as a search term in the URL. This version of `PageSearch` reflects both of its fields and renders new results through a `QUERY` action:

```go title="app/app.go"
// PageSearch is /search
type PageSearch struct{ App *App }

func (p PageSearch) GET(
	r *http.Request,
	query datapages.Query[struct {
		Term    string `query:"q" reflectsignal:"term"`
		InStock bool   `query:"stock" reflectsignal:"stock"`
	}],
) (body datapages.Component, err error) {
	results := p.App.catalog.Search(query.Values.Term, query.Values.InStock, 1)
	return searchPage(results), nil
}

// QUERYResults is /search/results
func (p PageSearch) QUERYResults(
	r *http.Request,
	sse datapages.SSE,
	signals datapages.Signals[struct {
		Term    string `json:"term"`
		InStock bool   `json:"stock"`
	}],
) error {
	results := p.App.catalog.Search(signals.Values.Term, signals.Values.InStock, 1)
	return sse.PatchElement(searchResults(results))
}
```

```templ title="app/app.templ"
templ searchPage(results []Product) {
	<main>
		<input
			type="search"
			data-bind:term
			data-on:input__debounce.300ms={ action.PageSearch.Results.QUERY() }
		/>
		<label>
			<input
				type="checkbox"
				data-bind:stock
				data-on:change={ action.PageSearch.Results.QUERY() }
			/>
			In stock
		</label>
		@searchResults(results)
	</main>
}

templ searchResults(results []Product) {
	<ul id="results">
		for _, p := range results {
			<li><a href={ href.PageProduct(p.ID) }>{ p.Name }</a></li>
		}
	</ul>
}
```

For `/search/?q=lamp&stock=true`, `GET` writes `data-signals:term="'lamp'"` and `data-signals:stock="true"` on the `<body>` element, which fills in the input and checks the box. It also writes an effect that rewrites the URL with `history.replaceState` whenever `$term` or `$stock` changes. The address bar follows the form without adding history entries, and reloading or sharing the URL renders the same results. The effect writes only the reflected keys. It removes a key when its signal is empty, `0` or `false`, and keeps every other parameter and the fragment.

`datapages gen` rejects a reflected field that is neither a basic type nor an `encoding.TextMarshaler`, 2 fields that reflect the same signal, and a signal name with a period-separated step that does not start with a lowercase letter or `_`. When the handler also takes `datapages.Signals`, the signal must be declared there with a type that decodes the query value. A `string` field reflected into a `bool` signal fails.

## The GET handler

`GET` takes `r *http.Request` and returns `body datapages.Component`. Add parameters and return values as the page needs them. The generator matches both by type, which allows any order and any names.

| Parameter | Provides |
| --------- | -------- |
| `r *http.Request` | the request, required |
| `path datapages.Path[T]` | route variables, see [Path parameters](#path-parameters) |
| `query datapages.Query[T]` | query parameters, see [Query parameters](#query-parameters) |
| `signals datapages.Signals[T]` | the signals of a Datastar `@get`, which sends them in the `datastar` query parameter. A plain page load has none. |
| `session Session` | the visitor's session, see [Sessions and auth](sessions.md) |
| `pageCache datapages.PageCacheWriter` | writes the offline page cache, see [Offline and shims](offline.md) |
| `datapages.Dispatcher[EventXXX]` | publishes `EventXXX`, see [Events](events.md) |

`GET` cannot take `datapages.SSE`, `datapages.State[T]` or `stateID`. The document is not a stream, and [per-tab state](state.md) exists only after the page connects its stream.

| Return value | Effect |
| ------------ | ------ |
| `body datapages.Component` | the content of `<body>`, required |
| `head datapages.Head` | tags added to `<head>`, see [The document head](#the-document-head) |
| `redirect datapages.Redirect` | a redirect sent instead of the page when `URL` is not empty |
| `newSession datapages.NewSession[Data]` | signs the visitor in, see [Sessions and auth](sessions.md) |
| `closeSession datapages.CloseSession` | signs the visitor out when `true` |
| `enableBackgroundStreaming datapages.EnableBackgroundStreaming` | keeps the stream open in a hidden tab, see [Streams](#streams) |
| `disableRefreshAfterHidden datapages.DisableRefreshAfterHidden` | skips the reload when a hidden tab returns, see [Streams](#streams) |
| `err error` | fails the request, see [Errors](actions.md#errors) |

This `GET` of `PageProduct` redirects a replaced product permanently and gives every other product its own title:

```go title="app/app.go"
// PageProduct is /products/{id}
type PageProduct struct{ App *App }

func (p PageProduct) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID int64 `path:"id"`
	}],
) (
	body datapages.Component,
	head datapages.Head,
	redirect datapages.Redirect,
	err error,
) {
	product, ok := p.App.catalog.Product(path.Values.ID)
	if !ok {
		return nil, nil, datapages.Redirect{}, datapages.ErrNotFound
	}
	if product.ReplacedBy != 0 {
		return nil, nil, datapages.Redirect{
			URL:    href.PageProduct(product.ReplacedBy),
			Status: http.StatusMovedPermanently,
		}, nil
	}
	return productPage(product), productHead(product), datapages.Redirect{}, nil
}
```

A `Redirect` with an empty `URL` renders the page. `Status` defaults to 302 Found, and a status that is not a redirect status becomes 302.

## The document head

The server writes the shell of every document. Its `<head>` starts with `<meta charset="UTF-8"/>`, the build script and the Datastar script. The output of `App.Head` follows, then the `head` that the handler returns, then the CSRF script for a visitor with a session. `App.Head` takes `r *http.Request` and optionally the session, and runs for every document:

```go title="app/app.go"
func (*App) Head(r *http.Request) datapages.Head {
	return globalHead()
}
```

```templ title="app/app.templ"
templ globalHead() {
	<meta name="viewport" content="width=device-width, initial-scale=1"/>
	<link rel="stylesheet" href={ assets.Path("style.css") }/>
}

templ productHead(p Product) {
	<title>{ p.Name } | Lamp Shop</title>
	<meta name="description" content={ p.Summary }/>
}
```

Put a viewport `<meta>` in `App.Head`, because the server writes none. A browser uses the first `<title>` of a document. When pages have their own titles, put the `<title>` in each page's head and not in `App.Head`. `assets.Path` requires [static assets](assets.md).

## Error and offline pages

`PageError404`, `PageError500` and `PageOffline` are reserved page names. Each is optional, declares its route like any other page and needs a `GET`:

```go title="app/app.go"
// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (body datapages.Component, err error) {
	return notFoundPage(), nil
}

// PageError500 is /error
type PageError500 struct{ App *App }

func (PageError500) GET(r *http.Request) (body datapages.Component, err error) {
	return errorPage(), nil
}
```

```templ title="app/app.templ"
templ notFoundPage() {
	<main>
		<h1>Page not found</h1>
		<p><a href={ href.PageIndex() }>Back to the shop</a></p>
	</main>
}
```

- `PageError404` renders with status 404 for a URL that no route matches and for a page load whose handler returns `datapages.ErrNotFound`.
- `PageError500` renders with status 500 for a page load whose handler panics or returns an error that wraps no sentinel.
- `PageOffline` is the page that the service worker shows offline for a URL it has no copy of. It renders with a zero session and must not depend on who is signed in. See [Offline and shims](offline.md#add-a-fallback-page).

Without these pages, the server answers with the status and its standard text, such as `Not Found`. The `GET` of `PageError404` and `PageError500` also answers its own route with status 200. It may take `session` to render the document for the visitor, but cannot return `newSession` or `closeSession`. Requests that Datastar sends, such as actions, do not render these pages. Their errors reach [`App.RecoverError`](actions.md#show-errors-in-the-page).

## Streams

A page gets an SSE stream when it handles an event with an `OnXXX` method, declares `StreamOpen` or `StreamClose`, or takes [per-tab state](state.md), itself or through a [stream embed](../SPECIFICATION.md#stream-embeds). Its document then carries a `data-init` attribute that connects to the page route plus `_$/`, and the event handlers patch the tab over that connection. A page without any of these has no stream. A page whose events are all addressed to users, and which has no per-tab state, opens its stream only for a signed-in visitor.

By default, the browser closes the stream while the tab is hidden, and the page reloads when the tab becomes visible again. The reload renders what changed while the stream was closed. 2 return values of `GET` change this:

- `enableBackgroundStreaming` set to `true` keeps the stream open while the tab is hidden and turns off the reload. Each hidden tab then holds its connection. Return `disableRefreshAfterHidden` set to `false` next to it to keep the reload.
- `disableRefreshAfterHidden` set to `true` turns off the reload. The stream still closes while the tab is hidden, and the page misses the events of that time.

`datapages gen` rejects both on a page without a stream. This dashboard keeps counting orders in a background tab:

```go title="app/app.go"
// EventOrderPlaced is "orders.placed"
type EventOrderPlaced struct {
	OrderID int64 `json:"order_id"`
}

// PageDashboard is /dashboard
type PageDashboard struct{ App *App }

func (p PageDashboard) GET(r *http.Request) (
	body datapages.Component,
	enableBackgroundStreaming datapages.EnableBackgroundStreaming,
	err error,
) {
	return dashboard(p.App.catalog.Stats()), true, nil
}

func (p PageDashboard) OnOrderPlaced(
	event EventOrderPlaced, sse datapages.SSE,
) error {
	return sse.PatchElement(dashboard(p.App.catalog.Stats()))
}
```

```templ title="app/app.templ"
templ dashboard(s Stats) {
	<main id="dashboard">
		<p>Orders today: { strconv.Itoa(s.Orders) }</p>
	</main>
}
```

See [Events](events.md) for event types and their delivery, and [Performance](performance.md#sse-compression) for the `StreamConfig` method that sets the compression of a page's streams.

## See also

- [Pages](../SPECIFICATION.md#pages): every page method, the route rules and the stream endpoints.
- [`datapages.Path`](../SPECIFICATION.md#parameter-datapagespathstruct-) and [`datapages.Query`](../SPECIFICATION.md#parameter-datapagesquerystruct-): field types, tags and `reflectsignal`.
- [`enableBackgroundStreaming`](../SPECIFICATION.md#get-return-value-enablebackgroundstreaming-datapagesenablebackgroundstreaming) and [`disableRefreshAfterHidden`](../SPECIFICATION.md#get-return-value-disablerefreshafterhidden-datapagesdisablerefreshafterhidden): streams of hidden tabs.
- [Actions](actions.md): methods that handle the requests a page sends.
- [Templates and forms](templates.md): links with the `href` package and Datastar attributes.
- [Events](events.md): update open pages over their streams.
