# Add items

An action handles a request from the page. Its name gives the HTTP method, and the first line of its doc comment gives the route. `POSTAdd` therefore handles `POST /items`. Add `"strings"` to the imports of `app/app.go`, then add these declarations:

```go title="app/app.go"
// Add appends an item named name to the list.
func (a *App) Add(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastID++
	a.items = append(a.items, Item{ID: a.lastID, Name: name})
}

// POSTAdd is /items
func (p PageIndex) POSTAdd(
	r *http.Request,
	sse datapages.SSE,
	signals datapages.Signals[struct {
		Name string `json:"name"`
	}],
) error {
	name := strings.TrimSpace(signals.Values.Name)
	if name == "" {
		return datapages.ErrBadRequest
	}
	p.App.Add(name)
	if err := sse.PatchElement(itemList(view{Items: p.App.Items()})); err != nil {
		return err
	}
	return sse.PatchSignals(map[string]string{"name": ""})
}
```

Datapages passes each parameter by its type:

- `signals` holds the Datastar signals, the client-side values that the browser sends with the request as JSON. The tag `json:"name"` reads the signal `name`.
- `sse` writes the response as server-sent events. `PatchElement` sends HTML, which Datastar morphs into the element with the same ID, here `#items`. `PatchSignals` sets `name` to `""`, which clears the input.

A blank name returns `datapages.ErrBadRequest`. An action that takes `sse` has already sent status 200 when it returns, and Datapages passes the error to `App.RecoverError`, which shows an alert.

Change `pageIndex` in `app/app.templ` and import the generated `action` package:

```templ title="app/app.templ"
import (
	"fmt"

	"example.com/shoplist/app/datapagesgen/action"
)

templ pageIndex(v view) {
	<main>
		<h1>Shopping list</h1>
		<form data-on:submit__prevent={ action.PageIndex.Add.POST() }>
			<input data-bind="name" required/>
			<button>Add</button>
		</form>
		@itemList(v)
	</main>
}
```

`data-bind="name"` binds the input to the signal `name`. `data-on:submit__prevent` runs the action when the form is submitted and prevents the browser's own submission. `action.PageIndex.Add.POST()` is the helper that `datapages gen` generates for `POSTAdd`, and it returns the Datastar expression `@post('/items/')`. `required` makes the browser refuse an empty input before it sends anything.

> [!IMPORTANT]
> Save `app/app.go` first and wait until the browser reloads. Then save `app/app.templ`.
>
> `datapages gen` type-checks the whole app package, `app/app_templ.go` included, and generates nothing while the package has a compile error. A template that calls `action.PageIndex.Add` before `datapages gen` has generated it is such an error, and it keeps `datapages gen` from generating the helper after you save `app/app.go`. Saving both files within a second of each other, as "Save All" does, has the same effect.
>
> This order works in every step because `app/app.go` compiles with the template of the step before.
>
> If the error page names `action.PageIndex` and stays after you save `app/app.go`, delete the call from `app/app.templ`, and its `action` import when no other call uses it. Save, wait for the reload, then restore both and save again.

Type "Milk" and press Enter. The item appears under the field, and the field clears. Type 3 spaces and press Enter, and the alert "Something went wrong. Please try again." appears.
