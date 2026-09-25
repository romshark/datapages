---
name: datapages-templates
description: >-
  Write Templ templates for a Datapages app: the generated href and action helpers,
  their options, attribute syntax and Templ pitfalls. Read with the datastar skill.
---

# Templates

Read `datapages` first for the build loop, hard rules and naming conventions.

A Templ component implements `datapages.Component`, so handlers can return it. Run `templ generate` to compile `.templ` files to `_templ.go`. Datapages does not run this command. Templ docs: https://templ.guide/llms.md

## href: links

The generated `href` package has one function for each page, named after its page type. Query structs are named `href.Query<PageType>`. Zero-value fields are omitted from the URL.

```templ
<a href={ href.PageIndex() }>Home</a>
<a href={ href.PagePost(post.Slug) }>{ post.Title }</a>
<a href={ href.PageMessages(href.QueryPageMessages{Chat: chatID}) }>Messages</a>
<a href={ href.Asset("logo.svg") }>Logo</a>
```

The linter rejects a hardcoded root-relative, relative, query-only or empty `href`. It also rejects `javascript:`. Literal `https://`, `mailto:`, `tel:`, `#frag` and `//cdn.example.com` values are valid. Use `href.External(url)` for an external URL computed by the app.

Inside `href={ ... }`, use an `href` package call, string literal or constant. The linter rejects variables and other calls, including `fmt.Sprintf` and `templ.SafeURL`, because it cannot resolve them.

## action: Datastar actions

An action helper is `action.<Owner>.<Name>.<METHOD>(...)`. `Owner` is the page type or `App`. `PageLogin.POSTSubmit` becomes `action.PageLogin.Submit.POST()`. `(*App).POSTSignOut` becomes `action.App.SignOut.POST()`. The helper returns the complete `@post('/...')` expression. After `datapages gen`, read the generated `datapagesgen/action` package for exact signatures.

```templ
<button data-on:click={ action.PageLogin.Submit.POST() }>Submit</button>
<button data-on:click={ action.PagePost.SendMessage.POST(slug) }>Send</button>
<button data-on:click={ action.App.SignOut.POST() }>Sign out</button>
```

A page action can be used only in that page's template. The linter rejects `action.PageA.X.POST()` in page B. App actions can be used on any page. Put an action expression in a Datastar action attribute, not in an `href`.

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

Each modifier returns `action.Option`. Collect conditional modifiers in an `[]action.Option` and pass `opts...`. Use this alias instead of importing `runtime/actionexpr`.

Pass arguments in this order: path variables in route order, the query value from `<METHOD>Query(...)`, then modifiers. The query type is unexported. Create it with the generated constructor.

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

`data-bind="email"` puts the signal name in the attribute value. HTML lowercases attribute names, so use this form for camelCase signals. For live validation, add `data-on:input__debounce.300ms` to an input and call a validation action on the same page.

An action sends signals by default. To send form controls or file data, use `action.WithContentType(action.ContentTypeForm)`. It selects the closest form. Add `action.WithSelector("#login")` only when the action must select another form. Form content type does not send signals.

## Syntax

- Call generated helpers through the Templ expression form: `data-on:click={ action.X() }`. Do not write `data-on:click="@post('/x/')"`.
- Event attributes use the colon form, such as `data-on:click` and `data-on:submit`. Plugin attributes use the hyphen form, such as `data-on-intersect`, `data-on-interval` and `data-on-signal-patch`.

## Pitfalls

- Put `//datapages:nolint` on the line above an element to suppress its attribute lint errors. You may add a reason after `//`. It does not suppress a cross-page action ownership error.
- Templ parses a text line that starts with `switch`, `if`, `for`, `else` or `case` as Go control flow, even inside HTML. Wrap the text in an element or reword it.
- An apostrophe in an attribute becomes `&#39;`. The browser decodes it before the JavaScript parser reads the expression, which can make the expression invalid. Reword it, use `&quot;` for inner strings or escape it with a backslash.
- Encode a value interpolated into `data-signals` with `json.Marshal` instead of quoting it yourself. An apostrophe in the value ends the string, and what follows it runs as script.

<!-- written by datapages v0.10.0 -->
