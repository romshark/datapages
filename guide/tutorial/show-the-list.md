# Show the list

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

The components take a `view` instead of the slice. Later steps add fields to `view` and keep the signatures of the components, which [Add items](add-items.md) relies on. Replace `app/app.templ`:

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
