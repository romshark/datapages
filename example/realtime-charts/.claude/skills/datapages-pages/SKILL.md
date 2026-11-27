---
name: datapages-pages
description: >-
  Add or change a Datapages page: route doc comments, GET return values,
  path and query parameters, custom error pages, the global head and
  sharing handlers across pages by embedding.
---

# Pages

Read `datapages` first for the build loop, hard rules and naming conventions.

Define 1 struct, 1 route doc comment and 1 `GET` method for each page. `PageIndex` for `/` is required. `App *App` is the only named field a page may declare. Put dependencies on `App`.

```go
// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return indexPage(), nil
}
```

Routes use `net/http.ServeMux` patterns. `/item/{id}` captures 1 segment. `/{path...}` captures the remaining path. `/{$}` matches only that path. Datapages serves a page's SSE stream below `_$`. Do not define a route that claims that path. A page whose route ends in `{$}` or `{name...}` serves only `GET`: it cannot have actions or an SSE stream. Leave out `{$}` to give a page actions or a stream: the page keeps its URL.

If a route comment has a description, separate it from the route with a blank `//` line:

```go
// PageItem is /item/{id}
//
// Shows 1 item.
```

## GET parameters

`r *http.Request` is required. A `GET` may also take `Session`, `Path`, `Query`, `Signals`, `datapages.PageCacheWriter` and dispatchers. It cannot take `datapages.SSE`, `datapages.State[T]` or `stateID`. See `datapages-actions` for the parameter types and `datapages-offline` for `pageCache`.

## GET return values

`(body datapages.Component, err error)` is the minimum. Add what you need, matched by type:

| type | effect |
| ---- | ------ |
| `datapages.Component` | the body |
| `datapages.Head` | extra `<head>` content for this page |
| `datapages.DocumentAttrs` | attributes of the `<html>` element by name |
| `datapages.Redirect` | `{URL, Status}`, sent instead of the body |
| `datapages.NewSession[Data]` | opens a session |
| `datapages.CloseSession` | ends the session |
| `datapages.EnableBackgroundStreaming` | keep the stream open while the tab is hidden; only a page with a stream may return it |
| `datapages.DisableRefreshAfterHidden` | no refresh when the tab is shown again; only a page with a stream may return it |
| `error` | reports an error |

A `GETXXX` method is a file action, not the page's `GET`. See
`datapages-actions`. The page's `GET` cannot return `datapages.File`.

## Path variables

```go
// PageItem is /item/{id}
func (PageItem) GET(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (body datapages.Component, err error) {
	return itemPage(path.Values.ID), nil
}
```

The `path` tag must match the `{...}` name in the route exactly.

## Query parameters

```go
query datapages.Query[struct {
	Term  string `query:"t"`
	Limit int    `query:"l"`
}]
```

Read values from `query.Values`. A `reflectsignal:"term"` tag binds a query field to a Datastar signal. The query parameter sets the initial signal value. Changing the signal rewrites the browser URL. Each segment of the period-separated path must start with a lowercase letter or underscore. Later characters may be letters, digits or underscores. A double underscore is invalid.

A signal change rewrites or removes only its own query parameter. All other parameters remain, including query fields without `reflectsignal`.

`datapages gen` rejects duplicate `reflectsignal` values. Both fields would emit the same `data-signals:term` attribute, and an HTML parser keeps only the first attribute.

A reflected field must be a basic type or implement `encoding.TextMarshaler`. A type that only implements `encoding.TextUnmarshaler` is rejected: it has no text form to seed the signal with.

It also rejects a JSON type mismatch between a query field and its signal. The checked types are number, boolean and string. For example, reflecting a `string` query field into a `bool` signal sets `$flag` to `"true"`. The next action sends `{"flag":"true"}`, which signal decoding rejects with 400.

## Special pages

`PageError404`, `PageError500` and `PageOffline` are reserved page names. All
3 are optional. Datapages serves defaults when they are absent.

`PageError404` renders with status 404 for a URL that no route matches and for a page load whose handler returns `datapages.ErrNotFound`. `PageError500` renders with status 500 for a page load whose handler panics or returns an error without a sentinel.

```go
// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (datapages.Component, error) {
	return notFoundPage(), nil
}
```

`PageError500` and `PageOffline` have the same form and each needs a `GET` method. `PageOffline` is the service worker fallback; see `datapages-offline`. The worker precaches 1 copy for every visitor, so the page renders without the CSRF script and must not depend on who is signed in.

The `GET` of `PageError404` and `PageError500` may not return `newSession` or `closeSession` because it serves both the page route and the error path. It may accept a session parameter to render the document for the visitor.

## Global head

```go
func (*App) Head(r *http.Request, session Session) datapages.Head {
	return globalHead()
}
```

`session` is optional. It applies to every page, so a per-page `head` return value only adds to it.

## The html element

`<html>` gets only the attributes returned by its `GET` handler. They are a `datapages.DocumentAttrs`, which is a `map[string]string`. There are no app-wide defaults.

If every request uses the same attributes, create 1 map and return it from each handler. Datapages only reads the map:

```go
var documentAttrs = datapages.DocumentAttrs{"lang": "en", "dir": "ltr"}

func (PageIndex) GET(r *http.Request) (
	body datapages.Component, docAttrs datapages.DocumentAttrs, err error,
) {
	return indexPage(), documentAttrs, nil
}
```

If the attributes have a few fixed combinations, such as 1 per theme, create those maps once and select the right one in the handler:

```go
var themeAttrs = map[string]datapages.DocumentAttrs{
	"light": {"lang": "en", "class": "light"},
	"dark":  {"lang": "en", "class": "dark"},
}

func (PageIndex) GET(r *http.Request, session Session) (
	body datapages.Component, docAttrs datapages.DocumentAttrs, err error,
) {
	return indexPage(), themeAttrs[session.Data().Theme], nil
}
```

Do not modify a shared map. Handlers run concurrently, so a write causes a data race. Clone the map first:

```go
	docAttrs = maps.Clone(themeAttrs[session.Data().Theme])
	docAttrs["lang"] = post.Lang
```

Create a map per request only when its values depend on the request, such as the visitor's language.

Datapages sorts attributes by name and writes empty values as empty attributes. A name must start with a lowercase ASCII letter and contain only lowercase ASCII letters, digits, `-`, `_`, `.` or `:`. `data-nonce` is reserved. An invalid name fails the page with 500. Page cache entries do not store attributes. A shim gets the current attributes when it hydrates.

## Sharing handlers

To share a stream hook or event handler, define it on a stream embed, a type without the `Page` prefix, and embed that type in each page. A stream embed is not a page and has no route. It needs the same `App *App` field. Its handlers run for each page that embeds it.

A stream embed cannot declare `GET` or actions. Give each page its own `GET`, which can call a shared function. Put an action that several pages call on `*App`, or define a separate action for each page.

```go
type Base struct{ App *App }

func (Base) OnMessageSent(event EventMessageSent, sse datapages.SSE) error {
	return sse.PatchElement(notification())
}

// PageChat is /chat
type PageChat struct {
	App *App
	Base
}
```

A method declared on a page replaces the embedded method only for that page. The replacement can call `p.Base.OnMessageSent(event, sse)` to wrap the embedded method.

<!-- written by datapages sha256:ce1395517b08483b -->
