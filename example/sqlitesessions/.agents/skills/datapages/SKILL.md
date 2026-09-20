---
name: datapages
description: >-
  Datapages framework rules, build loop and naming conventions, plus an index
  of the task skills. Activate for any work in a Datapages app package,
  its templates or its server entry point. `datapages-architecture` comes
  before this one when starting an app or designing a feature.
---

# Datapages

You write Go handlers and Templ templates. `datapages gen` writes the server: routing, handler wiring, SSE, sessions and the type-safe `href` and `action` packages.

Starting a new app, or designing a feature that is not written yet: read `datapages-architecture` first. It scaffolds the project and picks the constructs the task skills below then tell you how to write.

## Loop

Write the app model first, then generate its helpers before templates call them:

```sh
datapages gen
templ generate # after any .templ change; datapages does not run it
datapages lint
go build ./...
```

`datapages gen` reports parse errors with suggested fixes on stderr. Fix the app package and re-run. It also runs `go mod tidy`, whose failure makes the command fail even if generation succeeded. `datapages lint` checks without generating. If an earlier `templ generate` produced references to helpers that do not exist yet, remove those references, regenerate Templ, run `datapages gen`, then restore the references and regenerate Templ. On an initial parse failure, the generator may write empty stub helper packages. Use Templ `v0.3.1020`, the version pinned by the scaffolded CI workflow. `datapages watch` is a dev server for humans.

## Rules

- Never edit a `_gen.go` file, anything under `datapagesgen/`, or a file with a `DO NOT EDIT` header. Change the source and regenerate.
- Never hardcode an app-internal URL. `href.PageX()` for links, `action.PageX.Y.POST()` for page actions and `action.App.Y.POST()` for app actions.
- Never write JavaScript for application logic. Logic is Go on the server, the client is Datastar attributes. JS only for browser APIs Datastar cannot reach, such as the clipboard.
- Never open an SSE stream, set a CSRF header, add the Datastar script or register a service worker by hand. Datapages does all four.
- Submit `<form>` elements through Datastar actions. Browser form submissions do not carry the CSRF token; see `datapages-templates`.
- Do not put build-constrained files in the app package. The generator reads its pages, actions and events for the host platform, so a platform-specific declaration can disappear from generated code elsewhere.
- Prefer one HTML fragment that carries its own context over many small patches or over signal updates. The server is the source of truth, signals hold transient client state.

## Naming

The parser reads names and doc comments. Both decide behaviour.

| kind | name | doc comment |
| ---- | ---- | ----------- |
| page | `Page` + uppercase + alnum | `// PageX is /route` |
| action | `POST`/`PUT`/`PATCH`/`DELETE` + uppercase + alnum | `// POSTX is /route` |
| event | `Event` + uppercase + alnum | `// EventX is "subject.name"` |
| event handler | `On` + the event name after `Event`: `OnFoo` for `EventFoo` | none |
| stream hook | `StreamOpen`, `StreamClose` | none |
| assets | any `embed.FS` variable | `// StaticFS is /static/` |

No underscores, nothing lowercase after the prefix. The word `is` is required. Event subjects are quoted, routes are not. If a route comment has more description, put a blank `//` line after the first line before the description.

`PageIndex`, the page for `/`, is required. A page struct declares `App *App` and no other named field: embedded types are the only exception. Page methods take a value receiver, app-level methods (`Head`, `RecoverError`, app actions) a `*App`.

Handler parameters and return values are matched **by type**; order does not matter. Parameter names are free except `stateID`, which must use that name. Declare only what the handler needs.

## Testing

The generated server implements `http.Handler`. Use `httptest` to send requests through it. For a Datastar action, set `Datastar-Request: true`; for a stateful tab, carry the `Datapages-Instance` value from the page response into its action and stream requests. Assert the HTTP status and the returned HTML or SSE events, rather than only checking that `go build ./...` passes.

## Task skills

| skill | read it when |
| ----- | ------------ |
| `datapages-architecture` | a new app, or a feature whose constructs are not decided |
| `datapages-pages` | pages, routes, path and query parameters, error pages, `<head>` |
| `datapages-actions` | POST/PUT/PATCH/DELETE handlers, signals, SSE, errors |
| `datapages-events` | events, subjects, dispatchers, `On` handlers, stream hooks |
| `datapages-state` | per-tab state, state IDs and state-scoped events |
| `datapages-sessions` | authentication, session data, CSRF |
| `datapages-offline` | offline pages, cached shims, the service worker |
| `datapages-server` | the server entry point, options, broker, static assets |
| `datapages-templates` | `.templ` files, `href` and `action` helpers, Templ pitfalls |
| `datastar` | `data-*` attributes and `@get`/`@post` actions |
