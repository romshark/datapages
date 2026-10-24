# Sign in

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
