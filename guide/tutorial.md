# Tutorial

Build a shared shopping list with sign-in and an HTTP test. Every open tab shows the same list. A change in 1 tab updates all of them.

You need Go, templ v0.3.1070, the `datapages` CLI and Docker, which [Installation](installation.md) covers. The tutorial assumes you know Go and `net/http`. It introduces templ and Datastar where the code first uses them.

At the end, the app:

- shows the list in every open tab and updates each tab without a reload
- adds, checks off, renames and removes items
- keeps the rename field of 1 tab open while other tabs change the list
- lets signed-in users change the list and shows it read-only to guests
- passes a test that signs in and adds an item over HTTP

## 1. Create the project

Run `datapages init` in a directory outside any Git repository:

```sh
datapages init
```

Answer its questions like this:

| Question | Answer |
| -------- | ------ |
| Not inside a git repository. Create one? | Yes |
| Directory name | `shoplist` |
| No go.mod found. Initialize a Go module? | Yes |
| Module path | `example.com/shoplist` |
| Set up Web Components? | None |
| Run the app now? | No |

This command creates the same project without questions:

```sh
datapages init -n --name shoplist --module example.com/shoplist --web-components none
```

Start the development server:

```sh
cd shoplist
make dev
```

`make dev` starts NATS in Docker with `docker compose up -d`, then runs `datapages watch`. NATS carries the events and stores the sessions of the app. Answering Yes to "Run the app now?" runs `make dev` for you.

Open http://localhost:7331, the address of the dev server. The page shows "Welcome to Datapages". The app itself listens on `localhost:8080`, the default `PORT` of `cmd/server/main.go`, and the dev server forwards each request to it. Only a page loaded through `localhost:7331` reloads after a rebuild and shows build errors. `datapages watch` prints no URL of its own.

You edit 3 files:

- `app/app.go` declares the `App` type, the pages, their actions and the events, which together form the application model.
- `app/app.templ` holds the [templ](https://templ.guide) components that render the HTML.
- `cmd/server/main.go` creates the NATS broker and session manager and starts the server.

Do not edit `app/app_templ.go`, which templ writes, or `app/datapagesgen/`, which `datapages gen` writes.

Keep `make dev` running for the whole tutorial. When you save a file, `datapages watch` does this:

1. For `app/app.templ`, templ writes `app/app_templ.go`.
2. For a Go file in `app/` or `cmd/server/`, `app/app_templ.go` included, `datapages gen` rewrites `app/datapagesgen/`.
3. It builds and restarts the app, then reloads the open tabs.

When a step fails, the tabs reload and show an error page with the output of the failed step, such as `datapages gen` or `go`. The next successful build replaces it. [`datapages watch`](cli.md#datapages-watch) describes each step, and [datapages.yaml](configuration.md) its settings.

## 2. Show the list

The list lives on `App`. `cmd/server/main.go` creates 1 `App` that every request and every tab share, which is why a mutex guards the items. Replace `app/app.go`:

```go title="app/app.go"
package app

import (
	"net/http"
	"slices"
	"sync"

	"github.com/romshark/datapages"
)

// Item is 1 entry of the shopping list.
type Item struct {
	ID   int64
	Name string
}

type App struct {
	mu     sync.Mutex
	items  []Item
	lastID int64
}

// Items returns a copy of the list, which callers can read without the lock.
func (a *App) Items() []Item {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.items)
}

// Session carries no application data yet. Replace struct{} with your own type
// to keep data in the session, handlers read it with session.Data().
type Session = datapages.Session[struct{}]

func (*App) Head(r *http.Request) datapages.Head {
	return nil
}

func (*App) RecoverError(
	err error,
	sse datapages.SSE,
) error {
	return sse.ExecuteScript(`alert("Something went wrong. Please try again.")`)
}

// view is what the page renders for 1 tab.
type view struct {
	Items []Item
}

// PageIndex is /
type PageIndex struct{ App *App }

func (p PageIndex) GET(r *http.Request) (body datapages.Component, err error) {
	return pageIndex(view{Items: p.App.Items()}), nil
}

// PageError404 is /not-found
type PageError404 struct{ App *App }

func (PageError404) GET(r *http.Request, session Session) (body datapages.Component, err error) {
	return nil, nil
}

// PageError500 is /error
type PageError500 struct{ App *App }

func (PageError500) GET(r *http.Request) (body datapages.Component, err error) {
	return nil, nil
}
```

`PageIndex` is the page at `/`, which the first line of its doc comment declares. Its `GET` returns the templ component that renders the page body, and Datapages writes the rest of the document around it.

The components take a `view` instead of the slice. Later steps add fields to `view` and keep the signatures of the components, which [step 3](#3-add-items) relies on. Replace `app/app.templ`:

```templ title="app/app.templ"
package app

import "fmt"

templ pageIndex(v view) {
	<main>
		<h1>Shopping list</h1>
		@itemList(v)
	</main>
}

templ itemList(v view) {
	<div id="items">
		if len(v.Items) == 0 {
			<p>Nothing to buy.</p>
		}
		<ul>
			for _, item := range v.Items {
				<li id={ fmt.Sprintf("item-%d", item.ID) }>{ item.Name }</li>
			}
		</ul>
	</div>
}
```

Expressions in `{ }` are Go. Each item gets a stable element ID, and `#items` wraps the list. [Datastar](https://data-star.dev), the script that Datapages adds to every page, matches the HTML the server sends later with the elements of the page by these IDs.

Save `app/app.go`, then `app/app.templ`. After the first save the browser shows the `datapages gen` error `too many arguments in call to pageIndex`, because the new `GET` calls the old template. The second save clears it, and the page shows "Shopping list" and "Nothing to buy."

## 3. Add items

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

## 4. Update every open tab

`POSTAdd` patches the tab that sent the request. Another tab shows the new item only after a reload. To update every tab, the action dispatches an event, and every open page handles it. Add the event type and the handler to `app/app.go`, and replace `POSTAdd`:

```go title="app/app.go"
// EventItemsChanged is "items.changed"
type EventItemsChanged struct{}

// POSTAdd is /items
func (p PageIndex) POSTAdd(
	r *http.Request,
	sse datapages.SSE,
	signals datapages.Signals[struct {
		Name string `json:"name"`
	}],
	itemsChanged datapages.Dispatcher[EventItemsChanged],
) error {
	name := strings.TrimSpace(signals.Values.Name)
	if name == "" {
		return datapages.ErrBadRequest
	}
	p.App.Add(name)
	if err := itemsChanged.Dispatch(EventItemsChanged{}); err != nil {
		return err
	}
	return sse.PatchSignals(map[string]string{"name": ""})
}

func (p PageIndex) OnItemsChanged(
	event EventItemsChanged,
	sse datapages.SSE,
) error {
	return sse.PatchElement(itemList(view{Items: p.App.Items()}))
}
```

The doc comment of `EventItemsChanged` gives the subject `items.changed`, under which the broker publishes the event. The `datapages.Dispatcher` parameter publishes it. Each open tab keeps an SSE stream to its page, and `OnItemsChanged` runs once for each stream of `PageIndex` and patches that tab through its own `sse`.

The event carries no data. Each handler reads the current list from `App`, which keeps a tab correct after it has missed an event.

The tab that sent the request receives the event through its own stream as well. `POSTAdd` therefore no longer patches the list and only clears the input, which would otherwise send the list 2 times.

Save `app/app.go` and open http://localhost:7331 in a 2nd browser window. Add an item in 1 window, and it appears in both.

> [!NOTE]
> Put the 2 windows side by side instead of opening 2 tabs in 1 window. A hidden tab closes its stream and reloads when it becomes visible again, which also clears the per-tab state of step 6.

The rest of the tutorial calls the 2 windows tab A and tab B.

## 5. Check off and remove items

These actions act on 1 item, which the route names with the variable `{id}`. Add the field `Done` to `Item`, and add these declarations to `app/app.go`:

```go title="app/app.go"
type Item struct {
	ID   int64
	Name string
	Done bool
}

// Toggle flips the Done mark of the item id and reports whether it exists.
func (a *App) Toggle(id int64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	i := a.index(id)
	if i < 0 {
		return false
	}
	a.items[i].Done = !a.items[i].Done
	return true
}

// Remove deletes the item id and reports whether it existed.
func (a *App) Remove(id int64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	i := a.index(id)
	if i < 0 {
		return false
	}
	a.items = slices.Delete(a.items, i, i+1)
	return true
}

// index returns the position of the item id, or -1. The caller holds a.mu.
func (a *App) index(id int64) int {
	return slices.IndexFunc(a.items, func(it Item) bool { return it.ID == id })
}

// ItemPath is the route variable of the actions on 1 item.
type ItemPath struct {
	ID int64 `path:"id"`
}

// POSTToggle is /items/{id}/toggle
func (p PageIndex) POSTToggle(
	r *http.Request,
	path datapages.Path[ItemPath],
	itemsChanged datapages.Dispatcher[EventItemsChanged],
) error {
	if !p.App.Toggle(path.Values.ID) {
		return datapages.ErrNotFound
	}
	return itemsChanged.Dispatch(EventItemsChanged{})
}

// DELETEItem is /items/{id}
func (p PageIndex) DELETEItem(
	r *http.Request,
	path datapages.Path[ItemPath],
	itemsChanged datapages.Dispatcher[EventItemsChanged],
) error {
	if !p.App.Remove(path.Values.ID) {
		return datapages.ErrNotFound
	}
	return itemsChanged.Dispatch(EventItemsChanged{})
}
```

`datapages.Path[ItemPath]` parses `{id}` into the field with the tag `path:"id"`, and a value that is not an `int64` gets 400 Bad Request. Both actions return `datapages.ErrNotFound` when another tab removed the item first, and neither takes `sse`, because the event updates every tab, the sending one included.

The generated helpers take the route variable as a typed argument. `action.PageIndex.Toggle.POST(item.ID)` takes an `int64` and returns `@post('/items/3/toggle/')` for item 3. Replace the `<li>` element in `itemList` of `app/app.templ`:

```templ title="app/app.templ"
<li id={ fmt.Sprintf("item-%d", item.ID) }>
	<input
		type="checkbox"
		checked?={ item.Done }
		data-on:change={ action.PageIndex.Toggle.POST(item.ID) }
	/>
	if item.Done {
		<s>{ item.Name }</s>
	} else {
		{ item.Name }
	}
	<button data-on:click={ action.PageIndex.Item.DELETE(item.ID) }>Remove</button>
</li>
```

`checked?={ item.Done }` writes the attribute `checked` only for a done item. When the event re-renders the list, the morph sets the checkbox of every tab from that attribute.

Save `app/app.go`, wait for the reload, then save `app/app.templ`. Check off an item in tab A, and both tabs strike it through. Remove it in tab B, and both tabs drop it.

## 6. Rename an item with per-tab state

Renaming opens a field in 1 tab. `OnItemsChanged` renders the whole list in every tab. Without the ID of the item being renamed, an update from another tab would close the field. Event handlers receive no request or current signals. Keep the ID in `datapages.State[T]`, which gives each open tab its own server-side `*T` for the lifetime of its stream.

Add these declarations to `app/app.go`, and replace `view` and `OnItemsChanged`:

```go title="app/app.go"
// Rename sets the name of the item id and reports whether it exists.
func (a *App) Rename(id int64, name string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	i := a.index(id)
	if i < 0 {
		return false
	}
	a.items[i].Name = name
	return true
}

// view is what the page renders for 1 tab.
type view struct {
	Items   []Item
	Editing int64 // ID of the item the tab renames, 0 for none
}

// StateIndex is the per-tab state of PageIndex.
type StateIndex struct {
	Editing int64
}

func (p PageIndex) OnItemsChanged(
	event EventItemsChanged,
	sse datapages.SSE,
	state datapages.State[StateIndex],
) error {
	return sse.PatchElement(itemList(view{
		Items:   p.App.Items(),
		Editing: state.Values.Editing,
	}))
}

// POSTEdit is /items/{id}/edit
func (p PageIndex) POSTEdit(
	r *http.Request,
	sse datapages.SSE,
	path datapages.Path[ItemPath],
	state datapages.State[StateIndex],
) error {
	state.Values.Editing = path.Values.ID
	return sse.PatchElement(itemList(view{
		Items:   p.App.Items(),
		Editing: state.Values.Editing,
	}))
}

// POSTRename is /items/{id}/rename
func (p PageIndex) POSTRename(
	r *http.Request,
	path datapages.Path[ItemPath],
	signals datapages.Signals[struct {
		Rename string `json:"rename"`
	}],
	state datapages.State[StateIndex],
	itemsChanged datapages.Dispatcher[EventItemsChanged],
) error {
	name := strings.TrimSpace(signals.Values.Rename)
	if name == "" {
		return datapages.ErrBadRequest
	}
	if !p.App.Rename(path.Values.ID, name) {
		return datapages.ErrNotFound
	}
	state.Values.Editing = 0
	return itemsChanged.Dispatch(EventItemsChanged{})
}
```

`State[T]` takes an exported struct of the app package. Datapages allocates a zero `StateIndex` when a tab opens its stream and drops it when the stream closes. It runs the handlers of 1 tab that take the state one at a time, which is why `state.Values` needs no mutex.

`POSTEdit` changes only its own tab. It stores the ID and patches its tab through `sse`, without an event. `POSTRename` changes the shared list, clears `Editing` and dispatches the event, and the `OnItemsChanged` of its tab then renders the field closed.

In `app/app.templ`, replace the `<li>` element again:

```templ title="app/app.templ"
<li id={ fmt.Sprintf("item-%d", item.ID) }>
	if item.ID == v.Editing {
		<form
			data-signals:rename={ templ.JSONString(item.Name) }
			data-on:submit__prevent={ action.PageIndex.Rename.POST(item.ID) }
		>
			<input data-bind="rename" required/>
			<button>Save</button>
		</form>
	} else {
		<input
			type="checkbox"
			checked?={ item.Done }
			data-on:change={ action.PageIndex.Toggle.POST(item.ID) }
		/>
		if item.Done {
			<s>{ item.Name }</s>
		} else {
			{ item.Name }
		}
		<button data-on:click={ action.PageIndex.Edit.POST(item.ID) }>Rename</button>
		<button data-on:click={ action.PageIndex.Item.DELETE(item.ID) }>Remove</button>
	}
</li>
```

`data-signals:rename` sets the signal `rename` to the current name when the form appears. `templ.JSONString` encodes the name as a JSON string, which keeps a quote in a name from ending the expression. Datastar applies `data-signals` again only when its value changes, which keeps the text you typed when another tab re-renders the list.

Save `app/app.go`, wait for the reload, then save `app/app.templ`. Click Rename in tab A and type a new name without saving it. Add an item in tab B. Tab A shows the new item, and its field stays open with your text. Press Enter in tab A, and both tabs show the new name. A reload starts the tab with zero state, which closes the field.

## 7. Sign in

A session identifies the visitor by a cookie. Change `app/app.go`:

```go title="app/app.go"
import (
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/romshark/datapages"

	"example.com/shoplist/app/datapagesgen/href"
)

type Item struct {
	ID      int64
	Name    string
	Done    bool
	AddedBy string
}

// Add appends an item named name to the list.
func (a *App) Add(name, addedBy string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastID++
	a.items = append(a.items, Item{ID: a.lastID, Name: name, AddedBy: addedBy})
}

// SessionData is what the session keeps about the signed-in user.
type SessionData struct {
	Name string
}

type Session = datapages.Session[SessionData]

// view is what the page renders for 1 tab.
type view struct {
	Items   []Item
	Editing int64  // ID of the item the tab renames, 0 for none
	User    string // name of the signed-in user, empty for a guest
}

func (p PageIndex) GET(
	r *http.Request,
	session Session,
) (body datapages.Component, err error) {
	return pageIndex(view{
		Items: p.App.Items(),
		User:  session.Data().Name,
	}), nil
}

// POSTAdd is /items
func (p PageIndex) POSTAdd(
	r *http.Request,
	sse datapages.SSE,
	session Session,
	signals datapages.Signals[struct {
		Name string `json:"name"`
	}],
	itemsChanged datapages.Dispatcher[EventItemsChanged],
) error {
	if session.IsGuest() {
		return datapages.ErrForbidden
	}
	name := strings.TrimSpace(signals.Values.Name)
	if name == "" {
		return datapages.ErrBadRequest
	}
	p.App.Add(name, session.Data().Name)
	if err := itemsChanged.Dispatch(EventItemsChanged{}); err != nil {
		return err
	}
	return sse.PatchSignals(map[string]string{"name": ""})
}

func (p PageIndex) OnItemsChanged(
	event EventItemsChanged,
	sse datapages.SSE,
	state datapages.State[StateIndex],
	session Session,
) error {
	return sse.PatchElement(itemList(view{
		Items:   p.App.Items(),
		Editing: state.Values.Editing,
		User:    session.Data().Name,
	}))
}

// POSTToggle is /items/{id}/toggle
func (p PageIndex) POSTToggle(
	r *http.Request,
	session Session,
	path datapages.Path[ItemPath],
	itemsChanged datapages.Dispatcher[EventItemsChanged],
) error {
	if session.IsGuest() {
		return datapages.ErrForbidden
	}
	if !p.App.Toggle(path.Values.ID) {
		return datapages.ErrNotFound
	}
	return itemsChanged.Dispatch(EventItemsChanged{})
}

// DELETEItem is /items/{id}
func (p PageIndex) DELETEItem(
	r *http.Request,
	session Session,
	path datapages.Path[ItemPath],
	itemsChanged datapages.Dispatcher[EventItemsChanged],
) error {
	if session.IsGuest() {
		return datapages.ErrForbidden
	}
	if !p.App.Remove(path.Values.ID) {
		return datapages.ErrNotFound
	}
	return itemsChanged.Dispatch(EventItemsChanged{})
}

// POSTEdit is /items/{id}/edit
func (p PageIndex) POSTEdit(
	r *http.Request,
	sse datapages.SSE,
	session Session,
	path datapages.Path[ItemPath],
	state datapages.State[StateIndex],
) error {
	if session.IsGuest() {
		return datapages.ErrForbidden
	}
	state.Values.Editing = path.Values.ID
	return sse.PatchElement(itemList(view{
		Items:   p.App.Items(),
		Editing: state.Values.Editing,
		User:    session.Data().Name,
	}))
}

// POSTRename is /items/{id}/rename
func (p PageIndex) POSTRename(
	r *http.Request,
	session Session,
	path datapages.Path[ItemPath],
	signals datapages.Signals[struct {
		Rename string `json:"rename"`
	}],
	state datapages.State[StateIndex],
	itemsChanged datapages.Dispatcher[EventItemsChanged],
) error {
	if session.IsGuest() {
		return datapages.ErrForbidden
	}
	name := strings.TrimSpace(signals.Values.Rename)
	if name == "" {
		return datapages.ErrBadRequest
	}
	if !p.App.Rename(path.Values.ID, name) {
		return datapages.ErrNotFound
	}
	state.Values.Editing = 0
	return itemsChanged.Dispatch(EventItemsChanged{})
}

// POSTSignIn is /sign-in
func (*App) POSTSignIn(
	r *http.Request,
	signals datapages.Signals[struct {
		User string `json:"user"`
	}],
) (
	newSession datapages.NewSession[SessionData],
	redirect datapages.Redirect,
	err error,
) {
	name := strings.TrimSpace(signals.Values.User)
	if err := datapages.ValidateUserID(name); err != nil {
		return newSession, redirect, datapages.ErrBadRequest
	}
	newSession = datapages.NewSession[SessionData]{
		UserID: name,
		Data:   SessionData{Name: name},
	}
	return newSession, datapages.Redirect{URL: href.PageIndex()}, nil
}

// POSTSignOut is /sign-out
func (*App) POSTSignOut(r *http.Request) (
	closeSession datapages.CloseSession,
	redirect datapages.Redirect,
	err error,
) {
	return true, datapages.Redirect{URL: href.PageIndex()}, nil
}
```

`POSTSignIn` and `POSTSignOut` are methods of `*App`, which any page can call. Returning a `NewSession` signs the visitor in, and `CloseSession` set to `true` signs them out. `UserID` identifies the user, and `Data` keeps the name that the page shows. This sign-in accepts any name. A real app checks a password or another credential before it returns a `NewSession`.

The cookie travels in the response headers, which an action with `sse` has already sent. Both actions therefore take no `sse` and return a `Redirect`, and the browser loads the page again with the new cookie.

Each action that changes the list takes `session Session` and returns `datapages.ErrForbidden` to a guest. The template hides the controls from guests, but a request can reach an action without the page, and only the check in the action refuses it. Datapages also turns on CSRF protection for an app with a session type. A page rendered for a session carries a token, which Datastar sends with each action.

`OnItemsChanged` takes the session to fill `User`. A stream whose handlers read the session ends when the session closes, and its page reloads 1 second later.

The scaffold's `cmd/server/main.go` names the session data type `struct{}` in 4 places. Replace each with `app.SessionData`:

```go title="cmd/server/main.go"
	// in main
	opts = append(opts, datapages.WithSessionManager[app.SessionData](sessionManager))

	s, err := datapages.NewServer[
		app.App,
		app.SessionData,
		datapages.EnablePrometheus,
		datapagesgen.Server,
	](a, messageBroker, opts...)

// in connectNATS
func connectNATS() (
	*natscore.MessageBroker,
	*natskv.SessionManager[app.SessionData],
) {

	sessionManager, err := natskv.New[app.SessionData](
```

Replace `app/app.templ`:

```templ title="app/app.templ"
package app

import (
	"fmt"

	"example.com/shoplist/app/datapagesgen/action"
)

templ pageIndex(v view) {
	<main>
		<h1>Shopping list</h1>
		if v.User == "" {
			<form data-on:submit__prevent={ action.App.SignIn.POST() }>
				<input data-bind="user" placeholder="Your name" required/>
				<button>Sign in</button>
			</form>
		} else {
			<p>
				Signed in as { v.User }
				<button data-on:click={ action.App.SignOut.POST() }>Sign out</button>
			</p>
			<form data-on:submit__prevent={ action.PageIndex.Add.POST() }>
				<input data-bind="name" required/>
				<button>Add</button>
			</form>
		}
		@itemList(v)
	</main>
}

templ itemList(v view) {
	<div id="items">
		if len(v.Items) == 0 {
			<p>Nothing to buy.</p>
		}
		<ul>
			for _, item := range v.Items {
				<li id={ fmt.Sprintf("item-%d", item.ID) }>
					if item.ID == v.Editing {
						<form
							data-signals:rename={ templ.JSONString(item.Name) }
							data-on:submit__prevent={ action.PageIndex.Rename.POST(item.ID) }
						>
							<input data-bind="rename" required/>
							<button>Save</button>
						</form>
					} else {
						if v.User != "" {
							<input
								type="checkbox"
								checked?={ item.Done }
								data-on:change={ action.PageIndex.Toggle.POST(item.ID) }
							/>
						}
						if item.Done {
							<s>{ item.Name }</s>
						} else {
							{ item.Name }
						}
						<small>{ item.AddedBy }</small>
						if v.User != "" {
							<button data-on:click={ action.PageIndex.Edit.POST(item.ID) }>Rename</button>
							<button data-on:click={ action.PageIndex.Item.DELETE(item.ID) }>Remove</button>
						}
					}
				</li>
			}
		</ul>
	</div>
}
```

Save `app/app.go`. The browser shows a `go` error in `cmd/server/main.go`, because the generated server now expects `app.SessionData`. Save `cmd/server/main.go`, wait for the reload, then save `app/app.templ`.

Both tabs show the sign-in form. Sign in as "Alice" in tab A. It now shows Alice's name and the field for adding items. Tab B still shows the guest page, which has no CSRF token. Its actions receive 403 Forbidden because the browser now sends a session cookie. Reload tab B to show Alice's view there too.

Open a private window and sign in as "Bob" to create a 2nd user. Each item shows the name of the user who added it. Sign out in tab A. Tab B reloads as a guest 1 second later.

## 8. Test the app

The server that `datapages.NewServer` returns implements `http.Handler`, which `net/http/httptest` can serve without NATS or Docker. Create `app/app_test.go`:

```go title="app/app_test.go"
package app_test

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/messaging"
	msginmem "github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"

	"example.com/shoplist/app"
	"example.com/shoplist/app/datapagesgen"
)

// csrfToken finds the token in the script that adds the X-CSRF-Token header
// to the actions of a page rendered for a session.
var csrfToken = regexp.MustCompile(`"X-CSRF-Token",'([^']+)'`)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	s, err := datapages.NewServer[
		app.App,
		app.SessionData,
		datapages.EnablePrometheus,
		datapagesgen.Server,
	](
		&app.App{},
		msginmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithSessionManager[app.SessionData](sessinmem.New[app.SessionData](
			sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
		)),
		// Only ListenAndServe and ListenAndServeTLS open the metrics port.
		datapages.WithPrometheus(datapages.PrometheusConfig{Host: "127.0.0.1:0"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return srv
}

// TestAddItem tests that a guest cannot add an item and a signed-in user can.
func TestAddItem(t *testing.T) {
	srv := newServer(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Transport: srv.Client().Transport}

	post(t, client, srv.URL+"/items", "", `{"name":"Milk"}`)
	if strings.Contains(get(t, client, srv.URL+"/"), "Milk") {
		t.Fatal("a guest added an item")
	}

	post(t, client, srv.URL+"/sign-in", "", `{"user":"Alice"}`)
	m := csrfToken.FindStringSubmatch(get(t, client, srv.URL+"/"))
	if m == nil {
		t.Fatal("the page has no CSRF token after sign-in")
	}
	post(t, client, srv.URL+"/items", m[1], `{"name":"Milk"}`)

	if page := get(t, client, srv.URL+"/"); !strings.Contains(page, "Milk") {
		t.Errorf("the page does not list Milk:\n%s", page)
	}
}

func get(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// post sends a request the way Datastar sends an action: with the
// Datastar-Request header and the signals as a JSON body.
func post(t *testing.T, client *http.Client, url, csrfToken, signals string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(signals))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Content-Type", "application/json")
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s: %s", url, resp.Status)
	}
}
```

The test builds the server as `cmd/server/main.go` does, with the in-memory broker and session manager. It names the same 4 type arguments as `cmd/server/main.go`, because `datapages gen` reads every `NewServer` call of the module, test files included, and fails when 2 calls for 1 app name a different session type or metrics mode. `datapages.EnablePrometheus` requires `WithPrometheus`, and a server behind `httptest` opens no metrics port.

An action that takes signals or `sse` answers 406 Not Acceptable without the header `Datastar-Request: true`. The cookie jar keeps the session cookie between requests. After the sign-in, each action needs the CSRF token, which the test reads from the script of the page and sends in `X-CSRF-Token`. Without it the action answers 403 Forbidden.

The refused guest request still answers 200, because `RecoverError` handles the `ErrForbidden` of a Datastar request and writes the alert. The test therefore checks the page, not the status.

Run the tests:

```sh
go test ./...
```

`go test` reports `ok` for the package `example.com/shoplist/app`.

## Next steps

- [Events](events.md): subjects that address 1 user or 1 signal value, delivery and stream hooks.
- [Per-tab state](state.md): the lifecycle of state, events for 1 tab and the instance limit.
- [Sessions and auth](sessions.md): expiry, session managers and authorization.
- [Testing](testing.md): streams, per-tab state and signed-in clients in tests.
- [Deployment](deployment.md): building the binary, NATS and running several processes.
