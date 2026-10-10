---
name: datapages-actions
description: >-
  Write Datapages action handlers (GET, POST, PUT, PATCH, DELETE, QUERY): file
  responses, parameters, return values, Datastar signals, SSE patching, HTTP
  error status codes and the RecoverError hook.
---

# Actions

Read `datapages` first for the build loop, hard rules and naming conventions.

Define a page action as a value-receiver method on its page type. Define an action not tied to a page as a method on `*App`. Give each action one route doc comment. A page action route must be below its page route. For example, `/login/submit` is valid for `PageLogin` at `/login`. An action route cannot end in a `{name...}` wildcard. Pass that value in a query parameter or a signal.

```go
// POSTSubmit is /login/submit
func (PageLogin) POSTSubmit(r *http.Request) error { return nil }

// POSTSignOut is /sign-out/{$}
func (*App) POSTSignOut(r *http.Request) error { return nil }
```

`GETXXX` is a file action, not the page's `GET` handler. A browser loads it by
URL from a link, image or download. It must return `datapages.File` and may take
only `*http.Request`, `Session`, `Path` and `Query`. The generated `href` package
builds its URL. A GET action skips CSRF checks and must not change server state.

Use a `QUERYXXX` action for a read that sends data in the body, such as a search with a large filter. It skips the CSRF check because `QUERY` is a safe method. Never change state in it.

## Parameters

Parameters may appear in any order because the generator matches them by type. Names are unrestricted except for `stateID`. Read wrapped values from `.Values`.

| type | what |
| ---- | ---- |
| `*http.Request` | required |
| `datapages.SSE` | patches the stream of the calling page; page actions only, not `*App` actions |
| `Session` | the current session, see `datapages-sessions` |
| `datapages.Path[struct{...}]` | route variables, `path:"id"` tags |
| `datapages.Query[struct{...}]` | query parameters, `query:"p"` tags |
| `datapages.Signals[struct{...}]` | signals sent by the client, `json:"v"` tags |
| `datapages.State[StateX]` | per-tab server state, see `datapages-state` |
| `stateID string` | tab event address; requires `State[T]`, see `datapages-state` |
| `datapages.PageCacheWriter` | writes the offline page cache, see `datapages-offline` |
| `datapages.Dispatcher[EventX]` | publishes `EventX`, see `datapages-events` |

The name in a `Signals` field's `json` tag must match `[A-Za-z_][A-Za-z0-9_]*` and must not contain `__`. `json:"-"` is invalid. Omit a field to exclude it. Nested structs define signal paths.

## Return values

A non-GET action may return only `error`. Other supported return types are `datapages.Component`, `datapages.Head`, `datapages.Redirect`, `datapages.NewSession[Data]`, `datapages.CloseSession` and `datapages.File`. Return values may appear in any order.

```go
) (redirect datapages.Redirect, err error) {
	return datapages.Redirect{URL: href.PageIndex()}, nil
}
```

`Redirect.Status` defaults to 302. Datastar requests ignore it because they cannot follow an HTTP redirect. They navigate by assigning `window.location`.

Do not combine `datapages.SSE` with session changes. An `sse` parameter causes the response headers to be sent before the handler runs, so the handler cannot set or delete the session cookie. The generator rejects `newSession` or `closeSession` with `sse`. A `redirect` still works because it uses the stream. A returned `datapages.Component` is rejected with `sse` too: send it with `sse.PatchElement` instead.

## Files

A named GET action serves a file at its route:

```go
// GETImage is /images/{name}
func (a *App) GETImage(
	r *http.Request,
	path datapages.Path[struct {
		Name string `path:"name"`
	}],
) (datapages.File, error) {
	f, contentType, err := a.images.Open(path.Values.Name)
	if err != nil {
		return datapages.File{}, err
	}
	return datapages.File{Type: contentType, Body: f}, nil
}
```

Any action may return `datapages.File`. The file is the complete response, so it
may be accompanied only by `error`, which is optional. A file action cannot take
`datapages.SSE` or `datapages.PageCacheWriter`.

Do not use `datapages.File` to render application HTML. Return HTML as a Templ
`datapages.Component`. File bytes bypass Templ's contextual escaping and the
Datapages template linter.

## SSE

| method | effect |
| ------ | ------ |
| `Context()` | context of the SSE stream |
| `PatchElement(c)` | morph by element id |
| `PatchElementAt(c, sel, mode)` | target a selector; `datapages.PatchModeInner`, `...Replace`, `...Prepend`, `...Append`, `...Before`, `...After` |
| `RemoveElement(sel)` | remove matching elements |
| `PatchSignals(v)` | set client signals from JSON |
| `PatchSignalsIfMissing(v)` | set only signals that do not exist |
| `ExecuteScript(js)` | run JS in the browser |
| `Redirect(url)` | client-side navigation |
| `Prefetch(urls...)` | speculation rules hint |

A selector must not contain a line break. Prefer one complete fragment over several small targeted patches. `...Prepend` and `...Append` cannot recover missed events. See the delivery rules in `datapages-events`.

## Errors

```go
return datapages.ErrBadRequest                            // 400
return datapages.ErrForbidden                             // 403
return datapages.ErrNotFound                              // 404
return datapages.ErrConflict                              // 409
return fmt.Errorf("%w: %w", datapages.ErrNotFound, err)   // 404, keeps err
```

Any other error returns 500. The response body is the standard status text. A page load gets `PageError404` for 404 and `PageError500` for 500 instead when the app defines them. A Datastar request gets status 200 when the app defines `RecoverError`, and a handler with `sse` sends 200 before it runs. Wrap at most one sentinel. If an error contains several sentinels, the first of `ErrBadRequest`, `ErrForbidden`, `ErrNotFound` and `ErrConflict` sets the status.

## RecoverError

A Datastar request shows an HTTP error only in the console. Define this hook to patch an error message into the page.

```go
func (*App) RecoverError(err error, sse datapages.SSE) error {
	return sse.PatchElement(errorToast(err))
}
```

Only Datastar requests call this hook. It receives every handler error, including sentinels. Use `errors.Is` to identify sentinels. The response has status 200 whatever the error. For a page load, Datapages renders `PageError404` for a 404 and `PageError500` for a 500 if the app defines them. Otherwise it writes the status and its standard text if the response has not started. The hook writes an event stream, so do not use it as a page response.

A panic in a `GET`, action, `StreamOpen` or `On` handler becomes a `datapages.PanicError`. It contains the panic value and stack. Use `errors.As` to inspect it. Datapages logs the stack before it calls the hook, then ends the request.

`StreamClose` runs after the response completes. Datapages logs its panics but does not call the hook. If the hook returns an error, Datapages logs that error with the original one and does not change the response. Writing an HTTP error at that point would append plain text to the open SSE stream.

<!-- written by datapages sha256:ba783ddd93ec5497 -->
