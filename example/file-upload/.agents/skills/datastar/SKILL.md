---
name: datastar
description: >-
  Use Datastar data-* attributes and actions in HTML and Templ templates,
  including signals, bindings, events, server actions and request options.
  Use when writing template markup that uses Datastar.
---

# Datastar

Datastar provides client-side reactivity through `data-*` attributes. Datapages serves Datastar v1.0.4 and manages its script tag and SSE streams. Docs: https://data-star.dev/docs.md

Signals are reactive variables written as `$name`. In an expression, Datastar replaces `$signal` references and binds `el` to the current element. The sandbox allows calls only to `@name()` action helpers.

Use hyphens in signal keys. `data-signals:new-title` declares `$newTitle`. HTML lowercases attribute names, so `data-signals:newTitle` declares `$newtitle`. To preserve case without hyphens, put the name in an attribute value, as in `data-bind="newTitle"`.

Signal keys occur in `data-bind:*`, `data-signals:*`, `data-computed:*`, `data-indicator:*` and `data-ref:*`. Other keys remain in kebab-case. Use `__case.camel|kebab|snake|pascal` to override conversion. For example, `data-on:widget-loaded__case.camel` listens for `widgetLoaded`. A signal name must not contain `__`.

Datastar applies attributes depth first in DOM order. It applies them again when a patch changes them. Morphing preserves attributes that a patch does not change.

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

`data-on` modifiers: `__once`, `__passive`, `__capture`, `__prevent`, `__stop`, `__window`, `__document`, `__outside`, `__delay.500ms`, `__debounce.500ms[.leading|.notrailing]`, `__throttle.500ms[.noleading|.trailing]`, `__viewtransition`, `__case.*`.

## Actions

`@get(url, opts)`, `@post`, `@put`, `@patch` and `@delete` send a fetch request. In a Datapages app, use them through the generated `action` package. See `datapages-templates`.

`@peek(fn)` reads signals without subscribing to them. `@setAll(value, {include, exclude})` and `@toggleAll({include, exclude})` write every matching signal. The filters are regular expressions over signal paths.

Request options:

| option | values and effect |
| --- | --- |
| `contentType` | `json` sends signals; `form` sends the closest form or the form selected by `selector` |
| `filterSignals` | `{include, exclude}` regular expressions; signals prefixed with `_` are excluded by default |
| `selector` | selects the form for `contentType: form` |
| `headers` | adds request headers |
| `openWhenHidden` | defaults to false for GET and true for other methods |
| `payload` | sets a request payload expression |
| `retry` | `auto` for network errors, `error` for 4xx and 5xx, `always` or `never` |
| `retryInterval` | defaults to 1000 ms |
| `retryScaler` | defaults to 2 |
| `retryMaxWaitMs` | defaults to 30000 ms |
| `retryMaxCount` | defaults to 10 |
| `requestCancellation` | `auto`, `cleanup`, `disabled` or an `AbortController` |

`auto` cancels an active request from the same element. `cleanup` also cancels the request when the element or attribute is removed.

Datastar handles a response by content type:

| content type | result |
| --- | --- |
| `text/event-stream` | applies Datastar SSE events |
| `text/html` | patches elements |
| `application/json` | patches signals |
| `text/javascript` | executes the response |

Each request fires `datastar-fetch` events. `evt.detail.type` is `started`, `finished`, `error`, `retrying` or `retries-failed`.

## Practice

- Keep authoritative state on the server and transient UI state in signals. Use signals only when needed. Keep expressions to one statement and put application logic in Go.
- Patch elements instead of signals. Prefer one complete fragment over several small targeted updates.
- Encode a value with `json.Marshal` before putting it in an attribute, or wrap the element in `data-ignore`. The browser decodes Templ's escaping before the expression is parsed, hence an apostrophe in the value ends the string and what follows runs as script.
- Never put a secret in a signal. The browser can read signals, and requests send them to the server.
- Put `data-indicator` on the element that sends the request to show a loading state without JavaScript.
- Navigate with `<a href>`, not an action, so the browser keeps its history. Use default request options unless a requirement needs a different value.

A misused attribute logs `Uncaught datastar runtime error: <name>` with a link to a page explaining it.
