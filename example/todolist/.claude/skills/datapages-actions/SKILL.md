---
name: datapages-actions
description: >-
  Write Datapages action handlers (POST, PUT, PATCH, DELETE): parameters,
  return values, Datastar signals, SSE patching, HTTP error status codes and
  the RecoverError hook.
---

# Actions

Read `datapages` first for the build loop, hard rules and naming conventions.

Define a page action as a value-receiver method on its page type. Define an action not tied to a page as a method on `*App`. Give each action one route doc comment. A page action route must be below its page route. For example, `/login/submit` is valid for `PageLogin` at `/login`.

```go
// POSTSubmit is /login/submit
func (PageLogin) POSTSubmit(r *http.Request) error { return nil }

// POSTSignOut is /sign-out/{$}
func (*App) POSTSignOut(r *http.Request) error { return nil }
```

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
| `datapages.Dispatcher[EventX]` | publishes `EventX`, see `datapages-events` |

The name in a `Signals` field's `json` tag must match `[A-Za-z_][A-Za-z0-9_]*` and must not contain `__`. `json:"-"` is invalid. Omit a field to exclude it. Nested structs define signal paths.

## Return values

Returning only `error` is valid. Other supported return types are `datapages.Component`, `datapages.Head`, `datapages.Redirect`, `datapages.NewSession[Data]` and `datapages.CloseSession`. Return values may appear in any order.

```go
) (redirect datapages.Redirect, err error) {
	return datapages.Redirect{URL: href.PageIndex()}, nil
}
```

`Redirect.Status` defaults to 302. Datastar requests ignore it because they cannot follow an HTTP redirect. They navigate by assigning `window.location`.

Do not combine `datapages.SSE` with session changes. An `sse` parameter causes the response headers to be sent before the handler runs, so the handler cannot set or delete the session cookie. The generator rejects `newSession` or `closeSession` with `sse`. A `redirect` still works because it uses the stream.

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

Any other error returns 500. The response body is the standard status text. Wrap at most one sentinel. If an error contains several sentinels, the first of `ErrBadRequest`, `ErrForbidden`, `ErrNotFound` and `ErrConflict` sets the status.

## RecoverError

A Datastar request shows an HTTP error only in the console. Define this hook to patch an error message into the page.

```go
func (*App) RecoverError(err error, sse datapages.SSE) error {
	return sse.PatchElement(errorToast(err))
}
```

Only Datastar requests call this hook. It receives every handler error, including sentinels. Use `errors.Is` to identify sentinels. For a page load, Datapages renders `PageError500` if the app defines it. Otherwise it writes a plain HTTP error if the response has not started. The hook writes an event stream, so do not use it as a page response.

A panic in a `GET`, action, `StreamOpen` or `On` handler becomes a `datapages.PanicError`. It contains the panic value and stack. Use `errors.As` to inspect it. Datapages logs the stack before it calls the hook, then ends the request.

`StreamClose` runs after the response completes. Datapages logs its panics but does not call the hook. If the hook returns an error, Datapages logs that error with the original one and does not change the response. Writing an HTTP error at that point would append plain text to the open SSE stream.
