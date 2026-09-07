---
name: datapages
description: >-
  Datapages framework rules, build loop and naming conventions, plus an index
  of the task skills. Activate for any work in a Datapages app package,
  its templates or its server entry point.
---

# Datapages

You write Go handlers and Templ templates. `datapages gen` writes the server:
routing, handler wiring, SSE, sessions and the type-safe `href` and `action`
packages.

## Loop

```sh
templ generate        # after any .templ change, datapages does not run it
datapages gen         # after any app package change
go build ./...
```

`datapages gen` reports parse errors with suggested fixes on stderr. Fix the
app package and re-run. `datapages lint` checks without generating.
`datapages watch` is a dev server for humans.

## Rules

- Never edit a `_gen.go` file, anything under `datapagesgen/`, or a file with a
  `DO NOT EDIT` header. Change the source and regenerate.
- Never hardcode an app-internal URL. `href.PageX()` for links,
  `action.POSTPageXY()` for Datastar actions.
- Never write JavaScript for application logic. Logic is Go on the server, the
  client is Datastar attributes. JS only for browser APIs Datastar cannot reach,
  such as the clipboard.
- Never open an SSE stream, set a CSRF header or add the Datastar script by
  hand. Datapages does all three.
- Never use a plain HTML `<form>` submit. CSRF covers Datastar actions only.
- Prefer one HTML fragment that carries its own context over many small patches
  or over signal updates. The server is the source of truth, signals hold
  transient client state.

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

No underscores, nothing lowercase after the prefix. The word `is` is required.
Event subjects are quoted, routes are not.

`PageIndex`, the page for `/`, is required. A page struct declares `App *App`
and no other named field: embedded types are the only exception. Page methods
take a value receiver, app-level methods (`Head`, `RecoverError`, app actions)
a `*App`.

Handler parameters and return values are matched **by type**: names are free
and order does not matter, except that `error` comes last. Declare only what
the handler needs.

## Task skills

| skill | read it when |
| ----- | ------------ |
| `datapages-pages` | pages, routes, path and query parameters, error pages, `<head>` |
| `datapages-actions` | POST/PUT/PATCH/DELETE handlers, signals, SSE, errors |
| `datapages-events` | events, subjects, dispatchers, `On` handlers, stream hooks |
| `datapages-sessions` | authentication, session data, CSRF |
| `datapages-server` | the server entry point, options, broker, static assets |
| `datapages-templates` | `.templ` files, `href` and `action` helpers, Templ pitfalls |
| `datastar` | `data-*` attributes and `@get`/`@post` actions |
