# Check off and remove items

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
