---
name: datapages-pages
description: >-
  Add or change a Datapages page: route doc comments, GET return values,
  path and query parameters, custom error pages, the global head and
  sharing handlers across pages by embedding.
---

# Pages

Read `datapages` first for the build loop, hard rules and naming conventions.

Define one struct, one route doc comment and one `GET` method for each page. `PageIndex` for `/` is required. `App *App` is the only named field a page may declare. Put dependencies on `App`.

```go
// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return indexPage(), nil
}
```

Routes use `net/http.ServeMux` patterns. `/item/{id}` captures one segment. `/{path...}` captures the remaining path. `/{$}` matches only that path. Datapages serves a page's SSE stream below `_$`. Do not define a route that claims that path.

If a route comment has a description, separate it from the route with a blank `//` line:

```go
// PageItem is /item/{id}
//
// Shows one item.
```

## GET parameters

`r *http.Request` is required. A `GET` may also take `Session`, `Path`, `Query`, `Signals`, `datapages.PageCacheWriter` and dispatchers. It cannot take `datapages.SSE`, `datapages.State[T]` or `stateID`. See `datapages-actions` for the parameter types and `datapages-offline` for `pageCache`.

## GET return values

`(body datapages.Component, err error)` is the minimum. Add what you need, matched by type:

| type | effect |
| ---- | ------ |
| `datapages.Component` | the body |
| `datapages.Head` | extra `<head>` content for this page |
| `datapages.Redirect` | `{URL, Status}`, sent instead of the body |
| `datapages.NewSession[Data]` | opens a session |
| `datapages.CloseSession` | ends the session |
| `datapages.EnableBackgroundStreaming` | keep the stream open while the tab is hidden; only a page with a stream may return it |
| `datapages.DisableRefreshAfterHidden` | no refresh when the tab is shown again; only a page with a stream may return it |
| `error` | reports an error |

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
three are optional. Datapages serves defaults when they are absent.

```go
// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (datapages.Component, error) {
	return notFoundPage(), nil
}
```

`PageError500` and `PageOffline` have the same form and each needs a `GET`
method. Both render with a zero `Session`. `PageOffline` is the service worker
fallback; see `datapages-offline`.

Neither may return `newSession` or `closeSession` because the same `GET` serves the page route and its error path. Both may accept a session parameter, which an error page can use to render the document.

## Global head

```go
func (*App) Head(r *http.Request, session Session) datapages.Head {
	return globalHead()
}
```

`session` is optional. It applies to every page, so a per-page `head` return value only adds to it.

## Sharing handlers

To share a `GET`, stream hook or event handler, define it on a type without the `Page` prefix and embed that type in each page. The embedded type is not a page and has no route. It needs the same `App *App` field. Its handlers use the route of each page that embeds it.

Do not share an action by embedding it. Its doc comment defines one absolute route, so embedding it in a second page causes a route conflict. Put a shared action on `*App`, or define a separate action for each page.

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

<!-- written by datapages v0.10.0 -->
