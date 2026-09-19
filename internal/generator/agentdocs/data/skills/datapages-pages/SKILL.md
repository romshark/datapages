---
name: datapages-pages
description: >-
  Add or change a Datapages page: route doc comments, GET return values,
  path and query parameters, custom error pages, the global head and
  sharing handlers across pages by embedding.
---

# Pages

Read `datapages` first for the build loop, hard rules and naming conventions.

One struct per page, one route doc comment, one `GET` method. `PageIndex` for `/` is required. `App *App` is the only named field a page may declare, so per-page dependencies go on `App`.

```go
// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return indexPage(), nil
}
```

Routes are `net/http.ServeMux` patterns: `/item/{id}` captures a segment, `/{path...}` the rest, `/{$}` matches that path and nothing below it. `_$` is where a page's SSE stream is served. A route that claims it conflicts with that endpoint, so do not use it.

If a route comment has a description, separate it from the route with a blank `//` line:

```go
// PageItem is /item/{id}
//
// Shows one item.
```

## GET parameters

`r *http.Request` is required. A `GET` may also take `Session`, `Path`, `Query`, `Signals` and dispatchers. It cannot take `datapages.SSE`, `datapages.State[T]` or `stateID`. See `datapages-actions` for the parameter types.

## GET return values

`(body datapages.Component, err error)` is the minimum. Add what you need, matched by type:

| type | effect |
| ---- | ------ |
| `datapages.Component` | the body |
| `datapages.Head` | extra `<head>` content for this page |
| `datapages.Redirect` | `{URL, Status}`, sent instead of the body |
| `datapages.NewSession[Data]` | opens a session |
| `datapages.CloseSession` | ends the session |
| `datapages.EnableBackgroundStreaming` | keep the stream open while the tab is hidden |
| `datapages.DisableRefreshAfterHidden` | no refresh when the tab is shown again |
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

Values sit in `query.Values`. A `reflectsignal:"term"` tag binds the field to a Datastar signal: the parameter seeds the signal on load, and a signal change rewrites the browser URL. Its period-separated path must have each step start with a lowercase letter or underscore; later characters may be letters, digits or underscores. A double underscore is invalid.

A signal change rewrites or removes only its own parameter. Every other parameter remains, including a query field without `reflectsignal`.

`datapages gen` rejects two query fields with the same `reflectsignal` value. Both emit `data-signals:term`, and an HTML parser keeps only the first attribute, which drops the seed of the second field.

It also rejects a mismatch of JSON kind between the query field and the signal field, across number, boolean and string. A `string` query field reflected into a `bool` signal seeds `$flag` as `"true"`. The next action sends `{"flag":"true"}`, which signal decoding rejects with 400.

## Error pages

Optional. Without them Datapages serves plain error responses.

```go
// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request) (datapages.Component, error) {
	return notFoundPage(), nil
}
```

`PageError500` follows the same shape and needs a `GET` method too.

## Global head

```go
func (*App) Head(r *http.Request, session Session) datapages.Head {
	return globalHead()
}
```

`session` is optional. It applies to every page, so a per-page `head` return value only adds to it.

## Sharing handlers

Define a `GET`, a stream hook or an event handler once on a type without the `Page` prefix and embed it. Such a type is not a page and carries no route, but it needs the same `App *App` field. Their routes come from the page that embeds them, so any number of pages may.

An **action cannot be shared this way**: its doc comment names one absolute route, and the second page to embed it is rejected as a route conflict. Put a shared action on `*App` instead, or give each page its own.

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

A method declared on the page replaces the embedded one for that page only. Call `p.Base.OnMessageSent(event, sse)` from the override to wrap it.
