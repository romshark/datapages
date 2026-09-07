---
name: datastar
description: >-
  Datastar data-* attribute and action reference for HTML and Templ templates:
  signals, bindings, events, backend actions and their options.
  Activate when writing template markup that uses Datastar.
---

# Datastar

Frontend reactivity through `data-*` attributes. Datapages serves v1.0.3 and
manages the script tag and every SSE stream itself. Docs:
https://data-star.dev/docs.md

Signals are reactive variables written `$name`. An expression is JavaScript
with `$signal` substituted and `el` bound to the current element.
Actions are `@name()` helpers, the only calls the sandbox allows.

Casing: keys that name a signal (`data-bind:*`, `data-signals:*`,
`data-computed:*`, `data-indicator:*`, `data-ref:*`) become camelCase,
everything else kebab-case. `__case.camel|kebab|snake|pascal` overrides it,
so `data-on:widget-loaded__case.camel` listens for `widgetLoaded`.
A signal name may not contain `__`.

Attributes apply depth first in DOM order and are reapplied when a patch changes them.
Morphing preserves attributes it does not touch.

## Attributes

| attribute | effect |
| --------- | ------ |
| `data-attr:aria-label="$foo"` | sets an HTML attribute from an expression, kept in sync |
| `data-bind:name` | two-way binding on `input`, `select`, `textarea` and web components; keeps a predefined signal's type; `type=file` gives base64 |
| `data-class:font-bold="$x"` | adds or removes a class |
| `data-computed:foo="$a + $b"` | read-only derived signal |
| `data-effect="$a = $b"` | runs on load and whenever a signal it reads changes |
| `data-ignore` | skip the element and its subtree; `__self` for the element alone |
| `data-ignore-morph` | skip it while morphing |
| `data-indicator:fetching` | true while a fetch is in flight; put it on the element that triggers one |
| `data-init="@get('/x')"` | runs when the attribute initializes; `__delay.500ms` |
| `data-json-signals` | dumps signals as JSON for debugging; `__terse` |
| `data-on:click="$x = 1"` | event listener |
| `data-on-intersect` | on viewport intersection; `__once`, `__exit`, `__half`, `__full` |
| `data-on-interval` | repeatedly, default 1s; `__duration.500ms` |
| `data-on-signal-patch` | whenever signals are patched; narrow it with `data-on-signal-patch-filter` |
| `data-preserve-attr="open class"` | keeps those attributes across morphs |
| `data-ref:foo` | exposes the element as `$foo` |
| `data-show="$x"` | toggles `display` |
| `data-signals:count="0"` | declares signals, dot notation nests them; `null` removes one; `__ifmissing` keeps an existing value; a `_` prefix keeps it out of requests |
| `data-style:color="$c"` | sets a style property, falsy restores the original |
| `data-text="$x"` | sets the text content |

`data-on` modifiers: `__once`, `__passive`, `__capture`, `__prevent`, `__stop`,
`__window`, `__document`, `__outside`, `__delay.500ms`,
`__debounce.500ms[.leading|.notrailing]`,
`__throttle.500ms[.noleading|.trailing]`, `__viewtransition`, `__case.*`.

## Actions

`@get(url, opts)`, `@post`, `@put`, `@patch`, `@delete` send a fetch request.
In a Datapages app they always come from the generated `action` package,
see `datapages-templates`.

`@peek(fn)` reads signals without subscribing to them.
`@setAll(value, {include, exclude})` and `@toggleAll({include, exclude})` write
every matching signal, the filters being regexes over signal paths.

Options: `contentType` (`json` sends the signals, `form` sends the closest form
or the one named by `selector`), `filterSignals` (`{include, exclude}` regexes,
`_`-prefixed signals excluded by default), `selector`, `headers`,
`openWhenHidden` (false for GET, true otherwise), `payload`, `retry`
(`auto` on network errors, `error` on 4xx/5xx, `always`, `never`),
`retryInterval` 1000, `retryScaler` 2, `retryMaxWait` 30000, `retryMaxCount`
10, `requestCancellation` (`auto` cancels an in-flight request to the same URL
and method, `cleanup`, `disabled`, or an `AbortController`).

Responses dispatch on content type: `text/event-stream` (Datastar SSE events),
`text/html` (patch elements), `application/json` (patch signals),
`text/javascript` (execute).

Each request fires `datastar-fetch` events with `evt.detail.type` in `started`,
`finished`, `error`, `retrying`, `retries-failed`.

## Practice

- Server state on the server, transient UI state in signals.
  Use signals sparingly and keep expressions to a single statement: logic belongs in Go.
- Patch elements rather than signals, and trust the morph. One fragment that
  carries its context beats several surgical updates.
- Escape anything a user typed before it reaches an attribute, or wrap it in
  `data-ignore`.
- Never put a secret in a signal: signals travel to the backend and are
  readable in the browser.
- `data-indicator` on the element that fetches gives a loading state without JS.
- Navigate with an `<a href>`, not an action, and let the browser keep the
  history. Start from the default options and change one only for a reason.

A misused attribute logs `Uncaught datastar runtime error: <name>` with a link
to a page explaining it.
