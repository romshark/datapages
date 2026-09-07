---
name: datapages-actions
description: >-
  Write Datapages action handlers (POST, PUT, PATCH, DELETE): parameters,
  return values, Datastar signals, SSE patching, HTTP error status codes and
  the RecoverError hook.
---

# Actions

Methods on a page type (value receiver), or on `*App` for a route not tied to
a page. One route doc comment each.

```go
// POSTSubmit is /login/submit
func (PageLogin) POSTSubmit(r *http.Request) error { return nil }

// POSTSignOut is /sign-out/{$}
func (*App) POSTSignOut(r *http.Request) error { return nil }
```

## Parameters

Any order, matched by type, names free. Values sit in `.Values`.

| type | what |
| ---- | ---- |
| `*http.Request` | required |
| `datapages.SSE` | patches the stream of the calling page |
| `Session` | the current session, see `datapages-sessions` |
| `datapages.Path[struct{...}]` | route variables, `path:"id"` tags |
| `datapages.Query[struct{...}]` | query parameters, `query:"p"` tags |
| `datapages.Signals[struct{...}]` | signals sent by the client, `json:"v"` tags |
| `datapages.Dispatcher[EventX]` | publishes `EventX`, see `datapages-events` |

## Return values

`error` alone is valid. Otherwise pick from `datapages.Component`,
`datapages.Head`, `datapages.Redirect`, `datapages.NewSession[Data]`,
`datapages.CloseSession`, with `error` last.

```go
) (redirect datapages.Redirect, err error) {
	return datapages.Redirect{URL: href.PageIndex()}, nil
}
```

`Redirect.Status` defaults to 302 and is ignored for a Datastar request, which
cannot follow an HTTP redirect and navigates by assigning `window.location`.

**`datapages.SSE` and session mutation exclude each other.** Taking `sse` has
already sent the response headers the cookie would travel in, so `newSession`
and `closeSession` are rejected alongside it. `redirect` still works: it
navigates through the stream.

## SSE

| method | effect |
| ------ | ------ |
| `PatchElement(c)` | morph by element id |
| `PatchElementAt(c, sel, mode)` | target a selector; `datapages.PatchModeInner`, `...Replace`, `...Prepend`, `...Append`, `...Before`, `...After` |
| `RemoveElement(sel)` | remove matching elements |
| `PatchSignals(v)` | set client signals from JSON |
| `PatchSignalsIfMissing(v)` | set only signals that do not exist |
| `ExecuteScript(js)` | run JS in the browser |
| `Redirect(url)` | client-side navigation |
| `Prefetch(urls...)` | speculation rules hint |

A selector may not contain a line break. Prefer one fragment that carries its
own context over several surgical patches.

## Errors

```go
return datapages.ErrBadRequest                            // 400
return datapages.ErrForbidden                             // 403
return datapages.ErrNotFound                              // 404
return datapages.ErrConflict                              // 409
return fmt.Errorf("%w: %w", datapages.ErrNotFound, err)   // 404, keeps err
```

Any other error is 500. The response body is always the standard status text.
Wrap at most one sentinel; with several, the first of `ErrBadRequest`,
`ErrForbidden`, `ErrNotFound`, `ErrConflict` decides.

## RecoverError

An HTTP error on a Datastar request is invisible to the user: only the console
shows it. Define this hook to patch an error UI instead.

```go
func (*App) RecoverError(err error, sse datapages.SSE) error {
	return sse.PatchElement(errorToast(err))
}
```

Every handler error routes through it, sentinels included. Tell them apart
with `errors.Is`. A panic in a `GET`, an action, `StreamOpen` or an `On`
handler arrives as `datapages.PanicError` carrying the value and the stack
(`errors.As`); it is logged either way and the request ends there. An error
returned from the hook itself falls back to the plain HTTP error response for
the original error.
