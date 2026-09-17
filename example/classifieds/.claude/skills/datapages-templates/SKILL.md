---
name: datapages-templates
description: >-
  Write Templ templates for a Datapages app: the generated href and action helpers,
  their options, attribute syntax and Templ pitfalls. Read with the datastar skill.
---

# Templates

Read `datapages` first for the build loop, hard rules and naming conventions.

Handlers return `datapages.Component`, which a Templ component satisfies. `.templ` files compile to `_templ.go` through `templ generate`, which Datapages never runs for you. Templ docs: https://templ.guide/llms.md

## href: links

One function per page, named after the page type. Query structs are `href.Query<PageType>` and zero-value fields are left out of the URL.

```templ
<a href={ href.PageIndex() }>Home</a>
<a href={ href.PagePost(post.Slug) }>{ post.Title }</a>
<a href={ href.PageMessages(href.QueryPageMessages{Chat: chatID}) }>Messages</a>
<a href={ href.Asset("logo.svg") }>Logo</a>
```

A hardcoded root-relative, relative, query-only or empty `href` is a lint error, as is `javascript:`. A literal `https://`, `mailto:`, `tel:`, `#frag` or `//cdn.example.com` is fine, and `href.External(url)` covers one the app computes.

Inside `href={ ... }` only an `href` package call, a string literal or a constant is accepted. Any other call, `fmt.Sprintf` and `templ.SafeURL` included, and any variable, is rejected: the linter cannot resolve it.

## action: Datastar actions

An action is `action.<Owner>.<Name>.<METHOD>(...)`, where `Owner` is the page type or `App`. `PageLogin.POSTSubmit` becomes `action.PageLogin.Submit.POST()`; `(*App).POSTSignOut` becomes `action.App.SignOut.POST()`. It returns the whole `@post('/...')` expression. Read the generated `datapagesgen/action` package for exact signatures after `datapages gen`.

```templ
<button data-on:click={ action.PageLogin.Submit.POST() }>Submit</button>
<button data-on:click={ action.PagePost.SendMessage.POST(slug) }>Send</button>
<button data-on:click={ action.App.SignOut.POST() }>Sign out</button>
```

A page's action belongs to that page: using `action.PageA.X.POST()` in the template of page B is a lint error. App-level actions work anywhere. An action expression belongs in a Datastar action attribute, never in an `href`.

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

Arguments go in one order: the path variables as the route names them, then the query value built by `<METHOD>Query(...)`, then the modifiers. The query type is unexported, so build it with the generated constructor.

```templ
<button data-on:click={ action.PageMessages.Read.POST(
	action.PageMessages.Read.POSTQuery(msg.ID),
) }>Mark read</button>
```

## Forms

Use a Datastar submit action to keep Enter-to-submit and browser validation:

```templ
<form
	data-signals:email="''"
	data-signals:password="''"
	data-on:submit__prevent={ action.PageLogin.Submit.POST() }
>
	<input type="email" data-bind="email" required/>
	<input type="password" data-bind="password" required/>
	<button type="submit">Sign in</button>
</form>
```

`data-bind="email"` keeps the signal name in an attribute value. HTML lowercases attribute names, so use the value form for camelCase signals. For live validation, add `data-on:input__debounce.300ms` to an input and call a validation action on the same page.

The default action sends signals. To send form controls or file data instead, use `action.WithContentType(action.ContentTypeForm)`. It selects the closest form; add `action.WithSelector("#login")` only to target another form. Form content type sends no signals.

## Syntax

- Call generated helpers through the Templ expression form: `data-on:click={ action.X() }`, never `data-on:click="@post('/x/')"`.
- Event attributes take the colon form: `data-on:click`, `data-on:submit`. The hyphen form belongs to plugins: `data-on-intersect`, `data-on-interval`, `data-on-signal-patch`.

## Pitfalls

- `//datapages:nolint` on the line above an element suppresses its attribute lint errors, with an optional `// why` after it. It does not suppress the cross-page action ownership error.

- A text line starting with `switch`, `if`, `for`, `else` or `case` is parsed as Go control flow even inside HTML. Wrap it in an element or reword it.
- `'` in an attribute becomes `&#39;`, which decodes before the JavaScript parser sees it and breaks the expression. Reword it, use `&quot;` for inner strings, or escape it with a backslash.
