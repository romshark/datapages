---
name: datapages-templates
description: >-
  Write Templ templates for a Datapages app: the generated href and action helpers,
  their options, attribute syntax, Web Components and Templ pitfalls. Read with the
  datastar skill.
---

# Templates

Read `datapages` first for the build loop, hard rules and naming conventions.

A Templ component implements `datapages.Component`, so handlers can return it. Run `templ generate` to compile `.templ` files to `_templ.go`. Datapages does not run this command. Templ docs: https://templ.guide/llms.md

Return application HTML as a Templ `datapages.Component`. Do not render it into
a `datapages.File`: its bytes bypass Templ's contextual escaping and the
Datapages template linter.

## href: links

The generated `href` package has 1 function for each page, named after its page type. Query structs are named `href.Query<PageType>`. Zero-value fields are omitted from the URL.

```templ
<a href={ href.PageIndex() }>Home</a>
<a href={ href.PagePost(post.Slug) }>{ post.Title }</a>
<a href={ href.PageMessages(href.QueryPageMessages{Chat: chatID}) }>Messages</a>
<a href={ href.Asset("logo.svg") }>Logo</a>
```

The linter rejects a hardcoded root-relative, relative, query-only or empty `href`. It also rejects `javascript:`. Literal `https://`, `mailto:`, `tel:`, `#frag` and `//cdn.example.com` values are valid. Use `href.External(url)` for an external URL computed by the app.

Inside `href={ ... }`, use an `href` package call, string literal or constant. The linter rejects variables and other calls, including `fmt.Sprintf` and `templ.SafeURL`, because it cannot resolve them.

A `GETXXX` action, which answers with a `datapages.File`, has a URL builder that is a method of its owner: `(*App).GETImage` becomes `href.App.Image(name)`, `PageDoc.GETExport` becomes `href.PageDoc.Export(id)`. `href.PageDoc(id)` still builds the URL of the page. Pass a query value from the constructor next to the builder, as `href.App.Image(name, href.App.ImageQuery(width))`. Use these builders in `src` and `href`, such as `href.App.Image(img.Name)` for an image or `href.PageDoc.Export(doc.ID)` for a download link. The `action` package has no helper for a GET action: a browser loads it by URL.

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

## Web Components

Start with local signals and Datastar attributes. A local signal, named with a `_` prefix, holds interaction state such as an open menu or a selected tab, and requests do not carry it. A signal that an action sends, such as a form draft, has no prefix. To reuse markup and its attributes in several templates, make it a Templ component. Write a [Web Component](https://developer.mozilla.org/en-US/docs/Web/API/Web_components) when one of these holds:

- The code uses a library, such as a map or a rich text editor, or a browser API beyond 1 call, such as IndexedDB.
- The script builds DOM or keeps state that would be impractical in signals, such as text that runs ahead of the server.
- The logic needs more than 1 statement per expression. A Web Component also keeps the script in 1 place when several templates use it.

The element owns its script and the DOM inside it. The page owns the rest. Data goes down as attributes, and output goes up as events:

- **Down.** The template renders the attributes from current state. A morph changes them whenever the server renders different values, and `data-attr` changes one when its signal changes. List each attribute that can change in `observedAttributes` and apply it in `attributeChangedCallback`. The callback also runs for the initial values, before `connectedCallback`. Apply them again once the element is set up. Read an attribute that stays fixed for the life of the element, such as a configuration value, once in `connectedCallback`. Put data too large for an attribute in a `@templ.JSONScript` child and watch it with a `MutationObserver`: a morph replaces its text when the data changes.
- **Up.** The element dispatches a `CustomEvent` with the data in `detail`. `data-on:<event>` on the element turns the event into a generated action, and `action.WithPayload("evt.detail")` sends the data as the request body. A `CustomEvent` does not bubble by default, which is why the attribute goes on the element.
- **Commands.** A stream of changes does not fit an attribute. For one, the server calls a method of the element with `sse.ExecuteScript`. Wrap the call in `customElements.whenDefined("place-map").then(...)`, since the script can arrive before the bundle defines the element. Encode the argument with `json.Marshal`, which escapes `<` and `>`.

```templ
<place-map
	id="map"
	places={ placesJSON }
	selected={ selected }
	data-on:map-move={ action.PageSearch.ParamChange.QUERY(action.WithPayload("evt.detail")) }
>
	<ul>
		for _, p := range places {
			<li>{ p.Name }</li>
		}
	</ul>
</place-map>
```

```ts
// <place-map> shows places on a map. It owns the viewport and reports each move.
// createMap and LibraryMap stand for a map library.
class PlaceMap extends HTMLElement {
  static observedAttributes = ["places", "selected"];
  private map?: LibraryMap;

  connectedCallback() {
    if (this.map) {
      return;
    }
    // Morphs do not enter the shadow root. Without a slot, it also hides the
    // list the server rendered for the time before the bundle loads.
    this.map = createMap(this.attachShadow({ mode: "open" }));
    this.map.on("moveend", () =>
      this.dispatchEvent(new CustomEvent("map-move", { detail: this.map!.bounds() })),
    );
    for (const name of PlaceMap.observedAttributes) {
      this.attributeChangedCallback(name, null, this.getAttribute(name));
    }
  }

  // A morph or data-attr changed an attribute. The calls before
  // connectedCallback find no map.
  attributeChangedCallback(name: string, _old: string | null, value: string | null) {
    if (name === "places") {
      this.map?.setPlaces(JSON.parse(value ?? "[]"));
    } else {
      this.map?.highlight(value);
    }
  }
}

customElements.define("place-map", PlaceMap);
```

- The element never reads or changes DOM outside itself, calls an app route, opens an SSE stream, navigates or reloads the page. It dispatches an event, and a `data-on` attribute of the page acts on it.
- Render the initial content inside the element in Templ. It shows before the bundle loads, and a plain `GET` stays correct.
- A morph sets the attributes of the element to those the server rendered and morphs its children, but it does not enter a shadow root. Build the DOM the script owns in a shadow root, and give the element a stable `id`, which lets a morph match it to the element already in the page.
- Give each attribute 1 source: the template or `data-attr`. A morph removes an attribute the server's HTML lacks, including one that `data-attr` set, unless `data-preserve-attr` lists it.
- Add `data-ignore-morph` only when the script must build light DOM, for example for a library that does not work in a shadow root. A morph then skips the element and its attributes, when the old and the new element both carry `data-ignore-morph`. `data-attr` and `ExecuteScript` remain the ways down.
- By default, an event that starts a request cancels the active request of the same element. Keep the default when only the latest event matters, such as a viewport. Add `action.WithRequestCancellation(action.RequestCancellationDisabled)` when every event must reach the server, such as an edit.
- Bundle the scripts with esbuild into the assets directory and load them with `assets.Path`. A `custom-watchers` entry in `datapages.yaml` rebuilds them under `datapages watch`; see `datapages-server`. `datapages init --web-components typescript` scaffolds this setup. `example/webcomponents` and `example/docs` in the Datapages repository bundle their scripts this way.

## Syntax

- Call generated helpers through the Templ expression form: `data-on:click={ action.X() }`. Do not write `data-on:click="@post('/x/')"`.
- Event attributes use the colon form, such as `data-on:click` and `data-on:submit`. Plugin attributes use the hyphen form, such as `data-on-intersect`, `data-on-interval` and `data-on-signal-patch`.

## Pitfalls

- Put `//datapages:nolint` on the line above an element to suppress its attribute lint errors. You may add a reason after `//`. It does not suppress a cross-page action ownership error.
- Templ parses text that starts with `if `, `for ` or `switch ` as Go control flow, also at the start of the content of an element. Write the word as `{ "if" }` or reword the text.
- An apostrophe in an attribute becomes `&#39;`. The browser decodes it before the JavaScript parser reads the expression, which can make the expression invalid. Reword it, use `&quot;` for inner strings or escape it with a backslash.
- Encode a value interpolated into `data-signals` with `json.Marshal` instead of quoting it yourself. An apostrophe in the value ends the string, and what follows it runs as script.
