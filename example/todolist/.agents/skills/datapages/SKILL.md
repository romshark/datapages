---
name: datapages
description: >-
  Apply the Datapages framework rules, build loop and naming conventions, and
  select the relevant task skill. Use for work in a Datapages app package,
  template or server entry point. Read `datapages-architecture` first when
  starting an app or designing a feature.
---

# Datapages

Write Go handlers and Templ templates. `datapages gen` generates routing, handler registration, SSE and session code. It also generates the type-safe `href` and `action` packages.

Read `datapages-architecture` first when starting an app or designing a new feature. It explains how to select the required Datapages constructs. Then read the task-specific skills below.

## Loop

Write the app model first, then generate its helpers before templates call them:

```sh
datapages gen
templ generate # after any .templ change; datapages does not run it
datapages lint
go build ./...
```

`datapages gen` reports parse errors and suggested fixes on stderr. Fix the app package and run it again. It also runs `go mod tidy`. A tidy failure makes the command fail even when code generation succeeded. `datapages lint` performs the checks without generating files.

An earlier `templ generate` can produce references to helpers that do not exist yet. Remove those references, run `templ generate`, then run `datapages gen`. Restore the references and run `templ generate` again. After an initial parse failure, the generator may write empty stub helper packages.

Use Templ `v0.3.1020`, which the generated CI workflow pins. Use `datapages watch` as the local development server.

## Rules

- Never edit a `_gen.go` file, anything under `datapagesgen/`, or a file with a `DO NOT EDIT` header. Change the source and regenerate.
- Never hardcode an app-internal URL. `href.PageX()` for links, `action.PageX.Y.POST()` for page actions and `action.App.Y.POST()` for app actions.
- Never write JavaScript for application logic. Logic is Go on the server, the client is Datastar attributes. JS only for browser APIs Datastar cannot reach, such as the clipboard.
- Never open an SSE stream, set a CSRF header, add the Datastar script or register a service worker by hand. Datapages does all four.
- Submit `<form>` elements through Datastar actions. Browser form submissions do not carry the CSRF token; see `datapages-templates`.
- Do not put build-constrained files in the app package. The generator reads its pages, actions and events for the host platform, so a platform-specific declaration can disappear from generated code elsewhere.
- Prefer one HTML fragment that carries its own context over many small patches or over signal updates. The server is the source of truth, signals hold transient client state.

## Security

Datapages routes the request, hands the handler its session and escapes what Templ and the `href`, `action` and subject helpers write. The rest is on the application code:

- **Authorization.** The session says who the visitor is, never what they may see or do. Return `datapages.ErrForbidden` for authorization errors, wrapped or bare. The error message is logged.
- **Values in Datastar attributes.** The browser decodes the HTML escaping before the expression is parsed. Encode with `json.Marshal`, never with `fmt.Sprintf("'%s'", v)`. See `datapages-templates`.
- **`templ.Raw`.** Writes markup verbatim. Pass only markup the server built.
- **Hand-built URLs and subjects.** `href.PageX()` and a `datapages.Subject` field escape their values. A string you concatenate does not.
- **Offline snapshots.** `pageCache.Set` stores a page body in the browser's cache. It survives a sign-out until a handler clears it. Call `ClearAll()` on sign-in and sign-out. See `datapages-offline`.

A value Templ interpolates into text or into an attribute is already escaped. Escaping it again shows the escape sequence to the visitor.

`SECURITY.md` in the Datapages repository carries the full list, including what belongs to the deployment.

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

Do not use underscores or a lowercase letter after the prefix. The word `is` is required. Quote event subjects but not routes. If a route comment has more text, add a blank `//` line after its first line.

`PageIndex`, the page for `/`, is required. A page struct declares `App *App` and no other named field. It may also embed types. Page methods use value receivers. App methods such as `Head`, `RecoverError` and app actions use `*App`.

The generator matches handler parameters and return values by type, not by position. Parameter names are unrestricted except for `stateID`. Declare only the values that the handler needs.

## Testing

The generated server implements `http.Handler`. Test it by sending requests with `httptest`. Set `Datastar-Request: true` for a Datastar action. For a stateful tab, copy the `Datapages-Instance` value from the page response to its action and stream requests. Assert the HTTP status and returned HTML or SSE events. A successful `go build ./...` alone does not test request behavior.

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
