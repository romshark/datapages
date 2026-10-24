# Templates and forms

[templ](https://templ.guide) components render pages and the fragments patched by actions. Generated `href` and `action` packages build URLs and Datastar expressions. `datapages lint` checks their use in templates.

## Links with the href package

`datapages gen` writes 1 function per page except `PageError500` into the package `app/datapagesgen/href`, named after the page type. It takes the path variables in route order, then the query as a struct named `href.Query` plus the page type, such as `href.QueryPageSearch`:

```templ title="app/app.templ"
templ productCard(p Product) {
	<article>
		<img src={ href.Asset("logo.svg") } alt=""/>
		<a href={ href.PageProduct(p.ID) }>{ p.Name }</a>
		<a href={ href.PageSearch(href.QueryPageSearch{Term: p.Name, InStock: true}) }>
			Similar lamps in stock
		</a>
		<a href={ href.PageProduct.Reviews(p.ID) } download>Reviews as CSV</a>
	</article>
}
```

| Call | URL |
| ---- | --- |
| `href.PageProduct(42)` | `/products/42/` |
| `href.PageSearch(href.QueryPageSearch{Term: "Desk lamp", InStock: true})` | `/search/?q=Desk+lamp&stock=true` |
| `href.PageProduct.Reviews(42)` | `/products/42/reviews.csv` |
| `href.Asset("logo.svg")` | `/static/logo.svg` |

- A query field with the zero value stays out of the URL.
- A GET action gets a builder that is a method of its owner. `PageProduct.GETReviews` becomes `href.PageProduct.Reviews`, and `(*App).GETImage` would become `href.App.Image`. The query of a GET action comes from the constructor next to the builder, as `href.App.ImageQuery(width)`.
- `href.Asset` exists when the app package declares [static assets](assets.md).
- `href.External(url)` returns an external URL that the application computes at run time. Write a fixed external URL as a string literal.

The builders are Go functions. Renaming or removing a page, a path variable or a query field breaks the build of every template that links to it.

## Actions with the action package

The `action` package has 1 variable per owner, a field per action name and a method per HTTP method. `action.<Owner>.<Name>.<METHOD>(...)` returns the Datastar expression as a string:

| Call | Expression |
| ---- | ---------- |
| `action.PageProduct.Review.POST(42)` | `@post('/products/42/reviews/')` |
| `action.PageProduct.Review.DELETE(42, 7)` | `@delete('/products/42/reviews/7/')` |
| `action.PageSearch.Results.QUERY()` | `@query('/search/results/')` |
| `action.App.SignOut.POST()` | `@post('/sign-out/')` |

Put the call in a Datastar event attribute in templ's expression form, as `data-on:click={ action.App.SignOut.POST() }`. A page action may appear only in templates that its own page renders. An `App` action may appear in any template.

The arguments are the path variables in route order, then the query value, then options. The query type is unexported. Build it with the constructor named after the HTTP method plus `Query`, as the counter in [example/counter](../example/counter/) does for its `delta` parameter:

```templ
<button data-on:click={ action.PageIndex.Add.POST(action.PageIndex.Add.POSTQuery(1)) }>+</button>
```

### Options

Every helper takes options of type `action.Option`, which set the [Datastar request options](https://data-star.dev/reference/actions):

| Option | Sets |
| ------ | ---- |
| `action.WithContentType(action.ContentTypeForm)` | sends the closest form as form data instead of the signals |
| `action.WithSelector(selector)` | the form that `ContentTypeForm` sends |
| `action.WithFilterSignals(include, exclude)` | regular expressions that select the signals to send |
| `action.WithHeaders(map[string]string{...})` | additional request headers |
| `action.WithPayload(expr)` | a JavaScript expression sent as the body instead of the signals |
| `action.WithOpenWhenHidden(open)` | whether the request stays open in a hidden tab |
| `action.WithRetry`, `WithRetryInterval`, `WithRetryScaler`, `WithRetryMaxWaitMs`, `WithRetryMaxCount` | the retry policy |
| `action.WithRequestCancellation`, `WithRequestCancellationController` | whether a new request of the same element cancels the running one |
| `action.WithBefore(js)`, `action.WithAfter(js)` | JavaScript that runs before or after the call |
| `action.WithOption(key, value)` | any option, with a raw JavaScript value |

`WithOption` writes its key and value into the expression without validation or escaping. Use it only for an option without a typed helper, or for a value that the Go type of the helper cannot express, such as an unlimited retry count:

```templ title="app/app.templ"
<form
	data-on:submit__prevent={ action.PageProduct.Review.POST(p.ID,
		action.WithFilterSignals(`^review\.`, ""),
		action.WithOption("retryMaxCount", "Infinity"),
	) }
></form>
```

This renders `@post('/products/42/reviews/', {filterSignals: {include: /^review\./}, retryMaxCount: Infinity})`. To add options conditionally, collect them in an `[]action.Option` and pass it as `opts...`.

## Datastar attributes

Datastar keeps client state in signals, written `$name` in expressions. Its `data-*` attributes declare signals, bind them to inputs and update the page when they change. This review form uses the attributes that Datapages templates need most:

```templ title="app/app.templ"
templ reviewForm(p Product) {
	<form
		data-signals:review.rating="5"
		data-signals:review.text="''"
		data-indicator:_posting
		data-on:submit__prevent={ action.PageProduct.Review.POST(p.ID) }
	>
		<select data-bind="review.rating">
			<option value="5">5 stars</option>
			<option value="4">4 stars</option>
			<option value="3">3 stars</option>
			<option value="2">2 stars</option>
			<option value="1">1 star</option>
		</select>
		<textarea data-bind="review.text" maxlength="500"></textarea>
		<small data-text="$review.text.length + ' / 500'"></small>
		<button type="submit" data-attr:disabled="$_posting">Post review</button>
		<span data-show="$_posting">Posting...</span>
	</form>
}
```

| Attribute | Effect |
| --------- | ------ |
| `data-signals:review.rating="5"` | declares the signal `$review.rating` with the value `5` |
| `data-bind="review.text"` | keeps the textarea and `$review.text` in sync |
| `data-text` | sets the text of the element to the value of the expression |
| `data-show="$_posting"` | shows the element only while `$_posting` is true |
| `data-attr:disabled="$_posting"` | sets the `disabled` attribute from the expression |
| `data-indicator:_posting` | sets `$_posting` to true while a request of the form runs |
| `data-on:submit__prevent` | runs the action on submit and prevents the browser's own submission |

The [attribute reference](https://data-star.dev/reference/attributes) lists the other attributes and the modifiers, such as `__debounce.300ms`.

- The value of every Datastar attribute is a JavaScript expression. `data-signals:review.text="''"` declares an empty string.
- HTML lowercases attribute names. `data-signals:newTitle` declares `$newtitle`. Write `data-signals:new-title` to declare `$newTitle`, or put the name in the value, as `data-bind="newTitle"`.
- `data-bind` keeps the type of a signal declared earlier. The `<select>` sets `$review.rating` to the number `4`, not the string `"4"`, and the action decodes it into an `int`.
- Datastar leaves signals whose names start with `_` out of requests. Use the prefix for interaction state, such as `$_posting`.
- Never put a secret in a signal. The browser can read every signal, and requests send them to the server.
- Navigate with `<a href>`, not with an action. The browser then keeps its history.

## Forms

Submit a form through a Datastar action. `data-on:submit__prevent` keeps Enter-to-submit and the browser's own validation, which runs before the `submit` event:

```templ title="app/app.templ"
templ subscribeBox() {
	<div id="subscribe">
		<form
			data-signals:email="''"
			data-on:submit__prevent={ action.PageIndex.Subscribe.POST() }
		>
			<input type="email" data-bind="email" required/>
			<button type="submit">Subscribe</button>
			@subscribeError("")
		</form>
	</div>
}

templ subscribeError(msg string) {
	<p id="subscribe-error" role="alert">{ msg }</p>
}

templ subscribed(email string) {
	<div id="subscribe">
		<p>Thanks. We sent a confirmation to { email }.</p>
	</div>
}
```

The action checks the input again on the server:

```go title="app/app.go"
// POSTSubscribe is /subscribe
func (p PageIndex) POSTSubscribe(
	r *http.Request,
	sse datapages.SSE,
	signals datapages.Signals[struct {
		Email string `json:"email"`
	}],
) error {
	email := strings.TrimSpace(signals.Values.Email)
	if _, err := mail.ParseAddress(email); err != nil {
		return sse.PatchElement(subscribeError("Enter a valid email address."))
	}
	if err := p.App.newsletter.Subscribe(r.Context(), email); err != nil {
		return err
	}
	return sse.PatchElement(subscribed(email))
}
```

`PatchElement` matches each fragment by its `id`. `subscribeError` replaces the empty message element, and `subscribed` replaces the box with the form in it. For `a..b@example.com`, which the browser accepts and `mail.ParseAddress` rejects, the visitor sees the message of the server. A failure of `Subscribe` reaches [`App.RecoverError`](actions.md#show-errors-in-the-page). To validate while the visitor types, call a validation action from `data-on:input__debounce.300ms` on the input.

### Plain HTML forms

`datapages lint` rejects the `action` attribute of `<form>`. A plain form submission sends no `Datastar-Request` header and goes around the script that adds the CSRF token to Datastar requests:

- An action that takes `Signals`, `SSE` or `PageCacheWriter` answers it with 406.
- With sessions, the CSRF check answers 403 to the form of a signed-in visitor.
- Only the form of a guest reaches an action that takes none of these.

Send authenticated forms through Datastar actions. See [Plain Forms and CSRF](../SPECIFICATION.md#plain-forms-and-csrf).

## Web Components

Signals and Datastar attributes cover most interaction. Use a [Web Component](https://developer.mozilla.org/en-US/docs/Web/API/Web_components) for a client library such as a map or rich-text editor, for state that does not fit signals, or for logic that needs more than 1 statement per expression. [`datapages init --web-components`](cli.md#datapages-init) sets up a new project for Web Components.

Data goes into the element as attributes, and results come out as events:

```js title="app/static/star-rating.js"
// <star-rating value="3"> shows 5 stars and reports a click as a rating-change event.
class StarRating extends HTMLElement {
  static observedAttributes = ["value"];

  connectedCallback() {
    if (this.shadowRoot) {
      return;
    }
    const root = this.attachShadow({ mode: "open" });
    for (let i = 1; i <= 5; i++) {
      const star = document.createElement("button");
      star.type = "button";
      star.textContent = "\u2605";
      star.addEventListener("click", () =>
        this.dispatchEvent(new CustomEvent("rating-change", { detail: { rating: i } })),
      );
      root.append(star);
    }
    this.render();
  }

  // A morph that renders another value changes the attribute.
  attributeChangedCallback() {
    this.render();
  }

  render() {
    const value = Number(this.getAttribute("value"));
    this.shadowRoot?.querySelectorAll("button").forEach((star, i) => {
      star.ariaPressed = String(i < value);
    });
  }
}

customElements.define("star-rating", StarRating);
```

The template passes the rating down in `value` and turns the event into an action. `action.WithPayload("evt.detail")` sends the `detail` of the event, `{"rating":4}`, as the request body:

```templ title="app/app.templ"
templ ratingInput(p Product, rating int) {
	<script type="module" src={ assets.Path("star-rating.js") }></script>
	<star-rating
		id="rating"
		value={ strconv.Itoa(rating) }
		data-on:rating-change={ action.PageProduct.Rating.PUT(p.ID,
			action.WithPayload("evt.detail"),
		) }
	>
		{ strconv.Itoa(rating) } of 5 stars
	</star-rating>
}
```

The action reads the payload as signals:

```go title="app/app.go"
// PUTRating is /products/{id}/rating
func (p PageProduct) PUTRating(
	r *http.Request,
	sse datapages.SSE,
	session Session,
	path datapages.Path[struct {
		ID int64 `path:"id"`
	}],
	signals datapages.Signals[struct {
		Rating int `json:"rating"`
	}],
) error {
	if signals.Values.Rating < 1 || signals.Values.Rating > 5 {
		return datapages.ErrBadRequest
	}
	p.App.catalog.SetRating(path.Values.ID, session.UserID(), signals.Values.Rating)
	return sse.PatchElement(ratingSaved(signals.Values.Rating))
}
```

The element builds its buttons in a shadow root, which a morph does not enter. The text that the template renders inside the element shows until the script runs, and the shadow root hides it afterwards. Give the element a stable `id`, which lets a morph match it. The element must not call routes or change the page outside itself. [example/webcomponents](../example/webcomponents/) bundles 2 components with esbuild, which a custom watcher of `datapages watch` runs.

## Lint templates

`datapages lint` checks the `.templ` files of the app package and of the packages of its module that the app package imports. `datapages gen` applies the same checks. They report:

- an `<a>` whose `href` is a literal path, such as `href="/cart"`, or an expression other than an `href` package call, a string literal or a constant;
- `href.External` with a path of the application;
- a hardcoded Datastar action, such as `data-on:click="@post('/cart/add')"`;
- an action expression other than a plain `action` package call, or such a call concatenated with text;
- a `<form action=...>` attribute;
- an `action` call outside a Datastar action attribute, and an `href` call inside one;
- a page action in a template that another page renders.

An `href` may also hold a fragment, as `#reviews`, a protocol-relative URL, as `//cdn.example.com`, or an absolute URL with any scheme except `javascript:`. Each report names the file, line and column, with a fix:

```text
at app.templ:11:5: template uses relative href: /cart
fix: Use href={ href.PageCart(...) } instead of "/cart"

at app.templ:22:26: template uses action from another page: PageCart.Add.POST belongs to PageCart, used in PageIndex
fix: Move this action reference to a template used by PageCart, or use an action owned by PageIndex
```

Put `//datapages:nolint` on the line above an element to suppress the reports about its attributes. A reason may follow after `//`. The directive does not suppress the report of an action used on the wrong page:

```templ
//datapages:nolint // old bookmarks link here
<a href="/old-cart">Old cart</a>
```

## Pitfalls

- Templ reads text that starts with `if `, `for ` or `switch ` as Go code, also at the start of the content of an element. Write the word as `{ "if" }` or reword the text.
- Write event attributes with a colon, as `data-on:click`. The plugin attributes `data-on-intersect`, `data-on-interval` and `data-on-signal-patch` take a hyphen.
- Write `data-on:click={ action.X.Y.POST() }`. A hand-written `data-on:click="@post('/x/')"` fails lint and breaks when the route changes.
- Templ writes an apostrophe in an attribute as `&#39;`, and the browser decodes it before Datastar parses the expression. An apostrophe inside a single-quoted string then ends the string. Reword the text, use `&quot;` for the inner string, or escape the apostrophe with a backslash.

## Escape values in Datastar attributes

A value written into a Datastar attribute becomes JavaScript. Templ escapes it for HTML, but the browser decodes that escaping before Datastar evaluates the expression. Encode the value as JSON:

```templ title="app/app.templ"
templ greeting(name string) {
	<div data-signals:name={ templ.JSONString(name) }>
		<p data-text="'Hello, ' + $name"></p>
	</div>
}
```

`templ.JSONString` encodes the value with `encoding/json`, which quotes it, escapes quotes and backslashes, and writes `<` and `>` as Unicode escapes. Quoted by hand, as `data-signals:name={ "'" + name + "'" }`, the name `O'Brien` makes the expression invalid, and the name `x' + (document.title = 'pwned') + '` runs as script. Put server values into signals this way and use them as `$name` in other expressions, instead of writing them into the expressions. [SECURITY.md](../SECURITY.md) also names `templ.Raw`, which writes markup without escaping.

## See also

- [Linting](../SPECIFICATION.md#linting): every lint rule, the allowed `href` values and `//datapages:nolint`.
- [Plain Forms and CSRF](../SPECIFICATION.md#plain-forms-and-csrf): why authenticated forms need Datastar actions.
- [Datastar attributes](https://data-star.dev/reference/attributes) and [Datastar actions](https://data-star.dev/reference/actions): every attribute and request option.
- [Actions](actions.md): the handlers that templates call.
- [Pages and routes](pages.md): the routes and parameters that the `href` builders take.
- [example/webcomponents](../example/webcomponents/): Web Components bundled with esbuild.
