# Update every open tab

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
> Put the 2 windows side by side instead of opening 2 tabs in 1 window. A hidden tab closes its stream and reloads when it becomes visible again, which also clears the per-tab state that [Rename an item](rename-items.md) adds.

The rest of the tutorial calls the 2 windows tab A and tab B.
