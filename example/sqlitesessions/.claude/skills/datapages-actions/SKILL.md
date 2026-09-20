---
name: datapages-actions
description: >-
  Write Datapages action handlers (POST, PUT, PATCH, DELETE): parameters,
  return values, Datastar signals, SSE patching, HTTP error status codes and
  the RecoverError hook.
---

# Actions

Read `datapages` first for the build loop, hard rules and naming conventions.

Methods on a page type (value receiver), or on `*App` for a route not tied to a page. One route doc comment each. A page action route must be under its page route: for `PageLogin` at `/login`, `/login/submit` is valid.

```go
// POSTSubmit is /login/submit
func (PageLogin) POSTSubmit(r *http.Request) error { return nil }

// POSTSignOut is /sign-out/{$}
func (*App) POSTSignOut(r *http.Request) error { return nil }
```

## Parameters

Any order, matched by type. Names are free except `stateID`. Values sit in `.Values` where applicable.

| type | what |
| ---- | ---- |
| `*http.Request` | required |
| `datapages.SSE` | patches the stream of the calling page |
| `Session` | the current session, see `datapages-sessions` |
| `datapages.Path[struct{...}]` | route variables, `path:"id"` tags |
| `datapages.Query[struct{...}]` | query parameters, `query:"p"` tags |
| `datapages.Signals[struct{...}]` | signals sent by the client, `json:"v"` tags |
| `datapages.State[StateX]` | per-tab server state, see `datapages-state` |
| `stateID string` | tab event address; requires `State[T]`, see `datapages-state` |
| `datapages.PageCacheWriter` | writes the offline page cache, see `datapages-offline` |
| `datapages.Dispatcher[EventX]` | publishes `EventX`, see `datapages-events` |

The name in a `Signals` field's `json` tag must match `[A-Za-z_][A-Za-z0-9_]*` and cannot contain `__`. `json:"-"` is invalid; omit the field to exclude it. Nested structs define signal paths.

## Return values

`error` alone is valid. Otherwise pick from `datapages.Component`, `datapages.Head`, `datapages.Redirect`, `datapages.NewSession[Data]`, `datapages.CloseSession`. Return values may appear in any order.

```go
) (redirect datapages.Redirect, err error) {
	return datapages.Redirect{URL: href.PageIndex()}, nil
}
```

`Redirect.Status` defaults to 302 and is ignored for a Datastar request, which cannot follow an HTTP redirect and navigates by assigning `window.location`.

**`datapages.SSE` and session mutation exclude each other.** Taking `sse` has already sent the response headers the cookie would travel in, so `newSession` and `closeSession` are rejected alongside it. `redirect` still works: it navigates through the stream.

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

A selector may not contain a line break. Prefer one fragment that carries its own context over several surgical patches. `...Prepend` and `...Append` cannot recover missed events; see delivery rules in `datapages-events`.

## Errors

```go
return datapages.ErrBadRequest                            // 400
return datapages.ErrForbidden                             // 403
return datapages.ErrNotFound                              // 404
return datapages.ErrConflict                              // 409
return fmt.Errorf("%w: %w", datapages.ErrNotFound, err)   // 404, keeps err
```

Any other error is 500. The response body is always the standard status text. Wrap at most one sentinel; with several, the first of `ErrBadRequest`, `ErrForbidden`, `ErrNotFound`, `ErrConflict` decides.

## RecoverError

An HTTP error on a Datastar request is invisible to the user: only the console shows it. Define this hook to patch an error UI instead.

```go
func (*App) RecoverError(err error, sse datapages.SSE) error {
	return sse.PatchElement(errorToast(err))
}
```

Only a Datastar request reaches the hook. It receives every handler error, sentinels included. Tell them apart with `errors.Is`. A page load instead writes `PageError500` if the app defines one, otherwise a plain HTTP error if the response has not started. The hook writes an event stream, which a browser would render as the document.

A panic in a `GET`, an action, `StreamOpen` or an `On` handler arrives as `datapages.PanicError` carrying the value and the stack (`errors.As`). Datapages logs the stack before the hook runs and the request ends there. `StreamClose` runs after the response path. Its panics are only logged. An error returned from the hook itself falls back to the plain HTTP error response for the original error.
