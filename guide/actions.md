# Actions

An action is a method that handles a request from an open page. A Datastar attribute in a template usually sends the request. Actions can read typed input, patch the calling tab, return files and report HTTP errors.

## Declare an action

Name the method after an HTTP method and the action name, as in `POSTReview`. Put its route on the first line of its doc comment. A page action has a value receiver of its page type. Its route must be the page route or below it. This `POSTReview` belongs to `PageProduct` at `/products/{id}`:

```go title="app/app.go"
// POSTReview is /products/{id}/reviews
func (p PageProduct) POSTReview(
	r *http.Request,
	sse datapages.SSE,
	session Session,
	path datapages.Path[struct {
		ID int64 `path:"id"`
	}],
	signals datapages.Signals[struct {
		Review struct {
			Rating int    `json:"rating"`
			Text   string `json:"text"`
		} `json:"review"`
	}],
) error {
	if _, ok := p.App.catalog.Product(path.Values.ID); !ok {
		return datapages.ErrNotFound
	}
	review := signals.Values.Review
	if review.Rating < 1 || review.Rating > 5 {
		return datapages.ErrBadRequest
	}
	p.App.catalog.AddReview(path.Values.ID, session.UserID(), review.Rating, review.Text)

	if err := sse.PatchElement(reviewList(p.App.catalog.Reviews(path.Values.ID))); err != nil {
		return err
	}
	return sse.PatchSignals(map[string]any{
		"review": map[string]any{"rating": 5, "text": ""},
	})
}
```

A template calls it with the generated expression `action.PageProduct.Review.POST(p.ID)`. [Templates and forms](templates.md#datastar-attributes) shows the form that sends it. `datapages gen` reports an action whose route is outside its page route, and names the route to use:

```text
at app.go:38:17: action handler path is not under page path: PageCart.POSTAdd
fix: Use `// POSTAdd is /cart/add`
```

An action route cannot end in a `{name...}` wildcard. Pass such a value in a query parameter or a signal.

Declare an action that several pages call on `*App`. Its route does not have to be below a page, and every page may call it. An `App` action cannot take `datapages.SSE`:

```go title="app/app.go"
// POSTSignOut is /sign-out
func (*App) POSTSignOut(r *http.Request, session Session) (
	closeSession datapages.CloseSession,
	redirect datapages.Redirect,
	err error,
) {
	return true, datapages.Redirect{URL: href.PageIndex()}, nil
}
```

## HTTP methods

The method prefix sets the HTTP method of the route and of the generated helper. Choose it by what the action does:

- `POSTXXX` creates something or submits input.
- `PUTXXX` replaces a resource, and `PATCHXXX` changes part of one.
- `DELETEXXX` deletes a resource.
- `QUERYXXX` reads without changing anything and takes its input in the request body. Datastar sends it with `@query`, which puts the signals in the body. `QUERY` is a safe method, which is why the action skips the CSRF check and must not change server state. `PageSearch.QUERYResults` in [Pages and routes](pages.md#keep-the-url-in-sync-with-signals) is one.
- `GETXXX` serves a file that the browser loads by URL, from a link, an image or a download. It skips the cross-origin and CSRF checks and must not change server state. See [Serve a file](#serve-a-file).

With sessions, `POST`, `PUT`, `PATCH` and `DELETE` actions check a CSRF token. The script that Datapages writes into the document adds the token to every Datastar request with one of these methods.

Actions of 1 owner that share a name share 1 helper, with a method per HTTP method. `DELETEReview` next to `POSTReview` is `action.PageProduct.Review.DELETE(id, review)`:

```go title="app/app.go"
// DELETEReview is /products/{id}/reviews/{review}
func (p PageProduct) DELETEReview(
	r *http.Request,
	sse datapages.SSE,
	session Session,
	path datapages.Path[struct {
		ID     int64 `path:"id"`
		Review int64 `path:"review"`
	}],
) error {
	review, ok := p.App.catalog.Review(path.Values.Review)
	if !ok || review.ProductID != path.Values.ID {
		return datapages.ErrNotFound
	}
	if review.Author != session.UserID() {
		return datapages.ErrForbidden
	}
	p.App.catalog.DeleteReview(review.ID)
	return sse.RemoveElement("#review-" + strconv.FormatInt(review.ID, 10))
}
```

## Parameters

The generator matches parameters by type. They may appear in any order and with any name, except that the name `stateID` is reserved. Only `r *http.Request` is required:

| Parameter | Provides | Rules |
| --------- | -------- | ----- |
| `r *http.Request` | headers, cookies and the request context | |
| `signals datapages.Signals[T]` | the signals that Datastar sends in the body, or for `DELETE` in the `datastar` query parameter | [`Signals`](../SPECIFICATION.md#parameter-datapagessignalsstruct-) |
| `path datapages.Path[T]` | route variables, as on a [page](pages.md#path-parameters) | [`Path`](../SPECIFICATION.md#parameter-datapagespathstruct-) |
| `query datapages.Query[T]` | query parameters of the action URL | [`Query`](../SPECIFICATION.md#parameter-datapagesquerystruct-) |
| `session Session` | the visitor's session | [`Session`](../SPECIFICATION.md#parameter-session-datapagessessiondata) |
| `sse datapages.SSE` | patches for the calling tab, page actions only | [`SSE`](../SPECIFICATION.md#parameter-sse-datapagessse) |
| `datapages.Dispatcher[EventXXX]` | publishes `EventXXX` to the open pages | [`Dispatcher`](../SPECIFICATION.md#parameter-datapagesdispatchereventxxx) |
| `pageCache datapages.PageCacheWriter` | writes the offline page cache | [`PageCacheWriter`](../SPECIFICATION.md#parameter-pagecache-datapagespagecachewriter) |
| `state datapages.State[T]` | the per-tab state of the calling tab | [`State[T]`](../SPECIFICATION.md#parameter-datapagesstatet) |
| `stateID string` | the address of the calling tab for events, next to `state` | [`State[T]`](../SPECIFICATION.md#parameter-datapagesstatet) |

`datapages.Signals` decodes the JSON that Datastar sends in the body, or for `DELETE` in the `datastar` query parameter. Every field needs a `json` tag. A nested struct represents nested signals. For example, `Review` holds `$review.rating` and `$review.text`. A signal name matches `[A-Za-z_][A-Za-z0-9_]*` and contains no `__`.

For an action other than `DELETE`, the server reads at most 1 MiB of signals and answers 400 to a larger body. `datapages.WithBodySizeLimit` changes the limit. The limit does not apply to an action without `Signals`. A handler that reads `r.Body` must set its own limit, for example with `http.MaxBytesReader`.

An action that takes `Signals`, `SSE` or `PageCacheWriter` answers 406 to a request without the `Datastar-Request: true` header, such as a plain form submission.

See [Events](events.md) for dispatchers, [Per-tab state](state.md) for `State[T]`, [Sessions and auth](sessions.md) for sessions and [Offline and shims](offline.md) for the page cache.

## Patch the page

`datapages.SSE` sends patches to the tab that called the action. The response is an event stream, and Datastar applies each event as it arrives:

| Method | Effect |
| ------ | ------ |
| `PatchElement(c)` | morphs each top-level element of `c` into the element with the same `id` |
| `PatchElementAt(c, selector, mode)` | patches the elements that match a CSS selector, with a `datapages.PatchMode` such as `PatchModeInner` or `PatchModeAppend` |
| `RemoveElement(selector)` | removes the elements that match a CSS selector |
| `PatchSignals(v)` | sets signals from `v`, encoded as JSON |
| `PatchSignalsIfMissing(v)` | sets only the signals that the tab does not have |
| `ExecuteScript(js)` | runs a script in the tab |
| `Redirect(url)` | navigates the tab to `url` |
| `Prefetch(urls...)` | asks the browser to prefetch URLs through the Speculation Rules API |
| `Context()` | returns the context of the event stream |

`POSTReview` renders the whole review list again, patches it, then resets the form signals. The fragment carries the `id` that `PatchElement` matches:

```templ title="app/app.templ"
templ reviewList(reviews []Review) {
	<ul id="reviews">
		for _, r := range reviews {
			<li id={ "review-" + strconv.FormatInt(r.ID, 10) }>
				{ strconv.Itoa(r.Rating) }/5: { r.Text }
				<button
					type="button"
					data-on:click={ action.PageProduct.Review.DELETE(r.ProductID, r.ID) }
				>Delete</button>
			</li>
		}
	</ul>
}
```

The response to `{"review":{"rating":4,"text":"Bright enough"}}` carries 2 events:

```text
event: datastar-patch-elements
data: elements <ul id="reviews"><li id="review-1">4/5: Bright enough <button type="button" data-on:click="@delete(&#39;/products/42/reviews/1/&#39;)">Delete</button></li></ul>

event: datastar-patch-signals
data: signals {"review":{"rating":5,"text":""}}
```

Patch complete fragments, as `POSTReview` does, instead of several small targeted patches. A complete fragment also corrects what a tab missed. `DELETEReview` removes 1 element by selector. A selector must not contain a line break, and `RemoveElement` and `PatchElementAt` return `datapages.ErrSelectorLineBreak` for one. Encode a value for `ExecuteScript` with `json.Marshal`.

An `sse` parameter makes the server send status 200 and the event stream headers before the handler runs. The handler can no longer set the session cookie or answer with a document. `datapages gen` therefore rejects `newSession`, `closeSession`, `body` and `head` next to `sse`. A `redirect` still works because it travels over the stream.

`datapages.SSE` reaches only the calling tab. Dispatch an [event](events.md) to update the other tabs, also those on other server processes.

## Return values

An action may return `error` and the values below. Of these, only `redirect` combines with `sse`:

| Return value | Effect |
| ------------ | ------ |
| `redirect datapages.Redirect` | navigates to `URL` |
| `body datapages.Component` | answers with a complete HTML document |
| `head datapages.Head` | adds tags to the head of that document, requires `body` |
| `newSession datapages.NewSession[Data]` | signs the visitor in |
| `closeSession datapages.CloseSession` | signs the visitor out when `true` |
| `datapages.File` | answers with a file, allowed next to `error` only |

`POSTSignOut` above closes the session and redirects to the index page. A Datastar request cannot follow an HTTP redirect. The server answers it with status 200 and a `text/javascript` body, `window.location = "/";`, which Datastar runs. A plain form submission gets an HTTP redirect with `Redirect.Status`, 302 by default. See [Sessions and auth](sessions.md) for `newSession` and `closeSession`.

An action that returns `body` answers with a full document, as a page does. Datastar morphs it into the open document, and a browser shows it after a plain form submission.

### Serve a file

A `GETXXX` action returns a `datapages.File`. The browser loads it by URL, which the generated `href` package builds as a method of the owner. `href.PageProduct.Reviews(42)` returns `/products/42/reviews.csv`:

```go title="app/app.go"
// GETReviews is /products/{id}/reviews.csv
func (p PageProduct) GETReviews(
	r *http.Request,
	path datapages.Path[struct {
		ID int64 `path:"id"`
	}],
) (datapages.File, error) {
	if _, ok := p.App.catalog.Product(path.Values.ID); !ok {
		return datapages.File{}, datapages.ErrNotFound
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	for _, review := range p.App.catalog.Reviews(path.Values.ID) {
		_ = w.Write([]string{strconv.Itoa(review.Rating), review.Text})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return datapages.File{}, err
	}
	return datapages.File{
		Type: "text/csv; charset=utf-8",
		Body: memBody{bytes.NewReader(buf.Bytes())},
		Disposition: datapages.FileDisposition{
			Download: true,
			Filename: "reviews.csv",
		},
	}, nil
}

// memBody gives an in-memory file the Close that datapages.File requires.
type memBody struct{ *bytes.Reader }

func (memBody) Close() error { return nil }
```

The response carries `Content-Disposition: attachment; filename=reviews.csv`, which makes the browser save the file, and `Cache-Control: no-cache`, the header of the zero `Cache` field. `net/http.ServeContent` writes the body and handles `HEAD`, `Range` and, with `ModTime` or `ETag`, conditional requests. `Body` must support seeking, and Datapages closes it after serving.

A GET action takes only `r`, `session`, `path` and `query`. Other actions may return a `datapages.File` too. Serve a file that a visitor uploaded with `Disposition.Download`. Shown inline, an uploaded `text/html` or `image/svg+xml` file runs its scripts on the application's origin. Return application HTML as a templ component and never as a file, because templ escapes it and `datapages lint` checks it.

## Errors

Return a sentinel of the `datapages` package to answer with its status. Wrap it to keep the cause:

```go
return datapages.ErrBadRequest                          // 400
return datapages.ErrForbidden                           // 403
return datapages.ErrNotFound                            // 404
return datapages.ErrConflict                            // 409
return fmt.Errorf("%w: %w", datapages.ErrNotFound, err) // 404, keeps err
```

Any other error answers 500, and the server logs it. The body is the standard status text, such as `Forbidden`, whatever the error says. Wrap at most 1 sentinel per error. When an error wraps several, the first of `ErrBadRequest`, `ErrForbidden`, `ErrNotFound` and `ErrConflict` sets the status. A panic in an action becomes a `datapages.PanicError` with the panic value and stack. The server logs the stack and handles the panic like a returned error.

### Show errors in the page

Datastar shows the status of a failed request only in the browser console. Define `App.RecoverError` to patch a message into the page instead:

```go title="app/app.go"
func (*App) RecoverError(err error, sse datapages.SSE) error {
	msg := "Something went wrong. Try again."
	switch {
	case errors.Is(err, datapages.ErrForbidden):
		msg = "You may not do that."
	case errors.Is(err, datapages.ErrNotFound):
		msg = "This no longer exists. Reload the page."
	}
	return sse.PatchElement(errorBanner(msg))
}
```

```templ title="app/app.templ"
templ errorBanner(msg string) {
	<div id="error-banner" role="alert">{ msg }</div>
}
```

Each page renders `@errorBanner("")` where the message belongs. For a Datastar request, `RecoverError` receives the errors and panics of actions and of `StreamOpen`, sentinels included. Tell sentinels apart with `errors.Is` and a panic with `errors.As` and a `datapages.PanicError`. The response then has status 200. Page loads never reach `RecoverError`. They get the [error pages](pages.md#error-and-offline-pages).

An action with `sse` sends status 200 before it runs. Without `RecoverError`, its error reaches only the log.

## Call an action from a template

Put the generated expression in a Datastar event attribute, as `reviewList` does:

```templ
<button
	type="button"
	data-on:click={ action.PageProduct.Review.DELETE(r.ProductID, r.ID) }
>Delete</button>
```

It renders as `@delete('/products/42/reviews/1/')`. A page action may appear only in templates that its own page renders, and an `App` action in any template. See [Templates and forms](templates.md#actions-with-the-action-package) for arguments and options.

## Configure the SSE response

An action that answers with an event stream uses the SSE compression settings of the server. A method named `StreamConfig` followed by the action's method name, such as `StreamConfigPOSTReview`, replaces them for that action. See [Performance](performance.md#sse-compression).

## See also

- [Pages](../SPECIFICATION.md#pages): every action signature, the `Datastar-Request` requirement and the cross-origin check.
- [`datapages.SSE`](../SPECIFICATION.md#parameter-sse-datapagessse): the patch methods and the compression of event streams.
- [`datapages.File`](../SPECIFICATION.md#return-value-datapagesfile) and [GET Actions](../SPECIFICATION.md#get-actions): file responses, caching and downloads.
- [Return Value `error`](../SPECIFICATION.md#return-value-error-or-err-error) and [App](../SPECIFICATION.md#app): sentinels, `RecoverError` and panics.
- [Templates and forms](templates.md): call actions from templates and submit forms.
- [Events](events.md): update other tabs after an action.
