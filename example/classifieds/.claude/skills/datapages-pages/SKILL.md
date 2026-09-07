---
name: datapages-pages
description: >-
  Add or change a Datapages page: route doc comments, GET return values,
  path and query parameters, custom error pages, the global head and
  sharing handlers across pages by embedding.
---

# Pages

One struct per page, one route doc comment, one `GET` method. `PageIndex` for
`/` is required. `App *App` is the only named field a page may declare, so
per-page dependencies go on `App`.

```go
// PageIndex is /
type PageIndex struct{ App *App }

func (PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return indexPage(), nil
}
```

Routes are `net/http.ServeMux` patterns: `/item/{id}` captures a segment,
`/{path...}` the rest, `/{$}` matches that path and nothing below it. `_$` is reserved for the SSE
stream path and is rejected in a route.

## GET parameters

`r *http.Request` is required. The rest are the action parameters minus
`datapages.SSE`, which a `GET` may not take: `Session`, `Path`, `Query`,
`Signals` and dispatchers. See `datapages-actions` for the table.

## GET return values

`(body datapages.Component, err error)` is the minimum. Add what you need,
matched by type:

| type | effect |
| ---- | ------ |
| `datapages.Component` | the body, always first |
| `datapages.Head` | extra `<head>` content for this page |
| `datapages.Redirect` | `{URL, Status}`, sent instead of the body |
| `datapages.NewSession[Data]` | opens a session |
| `datapages.CloseSession` | ends the session |
| `datapages.EnableBackgroundStreaming` | keep the stream open while the tab is hidden |
| `datapages.DisableRefreshAfterHidden` | no refresh when the tab is shown again |
| `error` | always last |

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

Values sit in `query.Values`. A `reflectsignal:"term"` tag binds the field to a
Datastar signal: the parameter seeds the signal on load, and a signal change
rewrites the browser URL.

## Error pages

Optional. Without them Datapages serves plain error responses.

```go
// PageError404 is /not-found
type PageError404 struct{ App *App }
```

`PageError500` follows the same shape.

## Global head

```go
func (*App) Head(r *http.Request, session Session) datapages.Head {
	return globalHead()
}
```

`session` is optional. It applies to every page, so a per-page `head` return
value only adds to it.

## Sharing handlers

Define a `GET`, a stream hook or an event handler once on a type without the
`Page` prefix and embed it. Such a type is not a page and carries no route, but
it needs the same `App *App` field. Their routes come from the page that
embeds them, so any number of pages may.

An **action cannot be shared this way**: its doc comment names one absolute
route, and the second page to embed it is rejected as a route conflict. Put a
shared action on `*App` instead, or give each page its own.

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

A method declared on the page replaces the embedded one for that page only.
Call `p.Base.OnMessageSent(event, sse)` from the override to wrap it.
