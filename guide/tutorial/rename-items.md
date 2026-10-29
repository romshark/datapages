# Rename an item with per-tab state

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
