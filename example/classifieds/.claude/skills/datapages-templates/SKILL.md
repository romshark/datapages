---
name: datapages-templates
description: >-
  Write Templ templates for a Datapages app: the generated href and action helpers,
  their options, attribute syntax and Templ pitfalls. Read with the datastar skill.
---

# Templates

Handlers return `datapages.Component`, which a Templ component satisfies.
`.templ` files compile to `_templ.go` through `templ generate`, which Datapages
never runs for you. Templ docs: https://templ.guide/llms.md

## href: links

One function per page, named after the page type. Query structs are
`href.Query<PageType>` and zero-value fields are left out of the URL.

```templ
<a href={ href.PageIndex() }>Home</a>
<a href={ href.PagePost(post.Slug) }>{ post.Title }</a>
<a href={ href.PageMessages(href.QueryPageMessages{Chat: chatID}) }>Messages</a>
<a href={ href.Asset("logo.svg") }>Logo</a>
```

A hardcoded root-relative, relative, query-only or empty `href` is a lint
error, as is `javascript:`. A literal `https://`, `mailto:`, `tel:`, `#frag` or
`//cdn.example.com` is fine, and `href.External(url)` covers one the app
computes.

Inside `href={ ... }` only an `href` package call, a string literal or a
constant is accepted. Any other call, `fmt.Sprintf` and `templ.SafeURL`
included, and any variable, is rejected: the linter cannot resolve it.

## action: Datastar actions

One function per action handler: the method, then `Page` and the page name
after its `Page` prefix, then the handler name after its method prefix, so
`PageLogin.POSTSubmit` becomes `POSTPageLoginSubmit`. An app-level action drops
the page: `POSTAppSignOut`. It returns the whole `@post('/...')` expression.

```templ
<button data-on:click={ action.POSTPageLoginSubmit() }>Submit</button>
<button data-on:click={ action.POSTPagePostSendMessage(slug) }>Send</button>
<button data-on:click={ action.POSTAppSignOut() }>Sign out</button>
```

A page's action belongs to that page: using `action.POSTPageA...` in the
template of page B is a lint error. App-level actions work anywhere. An action
expression belongs in a Datastar action attribute, never in an `href`.

Every function takes variadic modifiers. Never hand-write the options object.

| modifier | argument |
| -------- | -------- |
| `action.WithContentType` | `action.ContentTypeJSON`, `action.ContentTypeForm` |
| `action.WithSelector` | CSS selector of the form to send |
| `action.WithFilterSignals` | include and exclude regexes, exclude may be empty |
| `action.WithHeaders` | `map[string]string` |
| `action.WithOpenWhenHidden` | `bool` |
| `action.WithPayload` | raw JavaScript expression |
| `action.WithRetry` | `action.RetryAuto`, `RetryError`, `RetryAlways`, `RetryNever` |
| `action.WithRetryInterval`, `WithRetryScaler`, `WithRetryMaxWaitMs`, `WithRetryMaxCount` | numbers |
| `action.WithRequestCancellation` | `action.RequestCancellationAuto`, `...Cleanup`, `...Disabled` |
| `action.WithRequestCancellationController` | expression holding an `AbortController` |
| `action.WithBefore`, `action.WithAfter` | JavaScript prepended or appended, joined with `"; "` |
| `action.WithOption` | raw key and value for anything the helpers miss |

Arguments go in one order: the path variables as the route names them, then
the query struct `action.Query<FunctionName>`, then the modifiers.

```templ
<button data-on:click={ action.POSTPageMessagesRead(
	action.QueryPOSTPageMessagesRead{MessageID: msg.ID},
) }>Mark read</button>
```

## Syntax

- Call generated helpers through the Templ expression form:
  `data-on:click={ action.X() }`, never `data-on:click="@post('/x/')"`.
- Event attributes take the colon form: `data-on:click`, `data-on:submit`. The
  hyphen form belongs to plugins: `data-on-intersect`, `data-on-interval`,
  `data-on-signal-patch`.

## Pitfalls

- `//datapages:nolint` on the line above an element suppresses its attribute
  lint errors, with an optional `// why` after it. It does not suppress the
  cross-page action ownership error.

- A text line starting with `switch`, `if`, `for`, `else` or `case` is parsed
  as Go control flow even inside HTML. Wrap it in an element or reword it.
- `'` in an attribute becomes `&#39;`, which decodes before the JavaScript
  parser sees it and breaks the expression. Reword it, use `&quot;` for inner
  strings, or escape it with a backslash.
