# Per-tab state

`datapages.State[T]` keeps server-side values for 1 browser tab. The tab's actions, event handlers and stream hooks can read and write those values. The stream controls the state's lifetime.

## What per-tab state is for

An event handler normally renders the same stored data for every tab of a page. Some fragments also depend on a value specific to 1 tab, such as a filter, selection or open editor. The handler cannot take current signals because the tab sends them only when its stream connects. Keep such values in per-tab state.

Keep durable data in a store. Per-tab state disappears with the tab's stream. Do not use it as a session store either. It belongs to 1 tab, not to a user, see [Sessions and auth](sessions.md). Values that the browser alone needs, such as an open menu, stay in local Datastar signals.

## Keep an editor open while the list changes

`PageIndex` lists items, and a tab can open 1 item in an editor. When any tab adds or renames an item, `OnItemsChanged` sends the whole list to every tab, as on [Events](events.md#handle-an-event). Without per-tab state that re-render would close the editor in a tab whose visitor is typing. The state remembers which item each tab edits:

```go title="app/app.go"
// StateIndex is the per-tab state of PageIndex.
type StateIndex struct {
	// Editing is the ID of the item this tab shows in the editor, or "".
	Editing string
}

// POSTEdit is /edit/{id}
func (p PageIndex) POSTEdit(
	r *http.Request,
	sse datapages.SSE,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	state datapages.State[StateIndex],
) error {
	item, ok := p.App.Items.Get(path.Values.ID)
	if !ok {
		return datapages.ErrNotFound
	}
	state.Values.Editing = item.ID
	if err := sse.PatchSignals(map[string]string{"title": item.Title}); err != nil {
		return err
	}
	return sse.PatchElement(itemList(p.App.Items.List(), item.ID))
}

// POSTSave is /save/{id}
func (p PageIndex) POSTSave(
	r *http.Request,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
	signals datapages.Signals[struct {
		Title string `json:"title"`
	}],
	state datapages.State[StateIndex],
	itemsChanged datapages.Dispatcher[EventItemsChanged],
) error {
	err := p.App.Items.Rename(r.Context(), path.Values.ID, signals.Values.Title)
	if err != nil {
		return err
	}
	state.Values.Editing = ""
	return itemsChanged.Dispatch(EventItemsChanged{})
}

func (p PageIndex) OnItemsChanged(
	event EventItemsChanged,
	sse datapages.SSE,
	state datapages.State[StateIndex],
) error {
	return sse.PatchElement(itemList(p.App.Items.List(), state.Values.Editing))
}
```

```templ title="app/app.templ"
templ itemList(list []items.Item, editing string) {
	<ul id="items">
		for _, it := range list {
			if it.ID == editing {
				<li id={ "item-" + it.ID }>
					<input data-bind="title"/>
					<button data-on:click={ action.PageIndex.Save.POST(it.ID) }>Save</button>
				</li>
			} else {
				<li id={ "item-" + it.ID }>
					{ it.Title }
					<button data-on:click={ action.PageIndex.Edit.POST(it.ID) }>Edit</button>
				</li>
			}
		}
	</ul>
}
```

`POSTEdit` puts the title into the `title` signal and records the item. Each later re-render in that tab renders the editor again, and the morph leaves the bound input and the text typed into it in place. `POSTSave` closes the editor of its own tab, and the event shows the new title in every tab. The other tabs keep their editors.

## Declare state

Declare `T` as an exported struct at the package level of the app package. `State[T]` rejects pointers, anonymous structs and types of other packages. A page holds state when one of its actions, event handlers or stream hooks takes `datapages.State[T]`. Every handler of the page, embedded ones included, must use the same `T`. Such a page gets a stream even without event handlers, because the stream bounds the lifetime of the state. `GET` cannot take state.

An action on `*App` may take `State[T]`. A tab of a page that uses another type, or none, receives `409 Conflict` from it, so keep an action that every page calls stateless.

## Lifecycle

State lives as long as the tab's stream:

1. `GET` mints a random ID, the instance ID, and sends it in the `Datapages-Instance` response header and in an inline script. It allocates nothing.
2. The page sends the ID with each Datastar request. When its stream connects, the server allocates a zero `T` before `StreamOpen` runs.
3. Before a stateful action or event handler, the server locks the tab's instance. The handlers of 1 tab run 1 at a time and need no locks of their own for `state.Values`.
4. When the stream closes, the server releases the instance.
5. A reconnect with the same ID allocates a new zero `T`. An action that arrives while no stream holds the instance receives `409 Conflict` with `Datapages-Retry: reconnect`, and the page reloads once.

By default Datastar closes the stream of a hidden tab, which releases its state, and Datapages reloads the page when the tab becomes visible again, which starts a new instance. A `GET` that returns `datapages.EnableBackgroundStreaming(true)` keeps the stream and the state alive while the tab is hidden.

Initialize state in `StreamOpen` when the zero value is wrong. It may read the signals the tab connects with, as [`example/todolist`](../example/todolist/) does for its filter and sort order. Keep what an action needs to act in the request itself. `POSTSave` takes the item ID from its path, not from `state.Values.Editing`, which a reconnect empties.

Do not keep `state.Values` after the handler returns. A goroutine the handler starts, a component rendered later and the `Body` of a returned `datapages.File` must not read it. Copy the fields they need.

## Address an event to 1 tab

A handler that takes `State[T]` may also take `stateID string`. An event with a `datapages.SubjectStateID` field set to that value reaches only that tab. This action starts an export in the background and tells the tab that asked for it when the export is done:

```go title="app/app.go"
// EventExportReady is "export.ready"
type EventExportReady struct {
	Tab datapages.SubjectStateID `json:"tab"`

	Rows int `json:"rows"`
}

// POSTExport is /export
func (p PageIndex) POSTExport(
	r *http.Request,
	_ datapages.State[StateIndex],
	stateID string,
	exportReady datapages.Dispatcher[EventExportReady],
) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), time.Minute)
	go func() {
		defer cancel()
		rows := p.App.export(ctx)
		err := exportReady.DispatchCtx(ctx, EventExportReady{
			Tab:  datapages.SubjectStateID(stateID),
			Rows: rows,
		})
		if err != nil {
			slog.Error("dispatching EventExportReady", slog.Any("err", err))
		}
	}()
	return nil
}

func (p PageIndex) OnExportReady(event EventExportReady, sse datapages.SSE) error {
	return sse.PatchElement(exportNotice(event.Rows))
}
```

The goroutine copies the string `stateID`, not the state. The state ID is a hash of the instance ID. It addresses events to the tab and gives no access to the tab's state. The `SubjectStateID` field must be the only subject field of the event and takes no `signal` tag. A page that handles the event must hold state, and cannot also handle user-addressed or signal-scoped events.

## Limit live instances

Each server holds at most `datapages.DefaultMaxConcurrentInstances` instances, 10,000, across all state types. Size the limit by the memory of `T`:

```go title="cmd/server/main.go"
opts = append(opts, datapages.WithStateConfig(datapages.StateConfig{
	MaxConcurrentInstances: 50_000,
}))
```

0 selects the default and a negative value removes the limit. A stream that would exceed it receives `503 Service Unavailable` with `Retry-After`. The page retries after 1 second and doubles the delay up to 30 seconds. The application is not notified. The generated `Server` reports the live count with `StateLiveInstances()`, and with Prometheus the `datapages_state_instances` gauge exports it. The limit applies to the whole server, so limit the connections of each client in a proxy or in middleware added with `datapages.WithMiddleware`, as [Security](security.md#limit-abuse) shows.

## Run several processes

State lives in the memory of 1 process. Route the stream and the actions of a tab to the same process, by a hash of the `Datapages-Instance` request header, by session or by an affinity cookie. `GET` may reach any process. A process accepts any well-formed instance ID, also one that another process minted, and creates the state where the stream connects. An action that reaches another process receives `409 Conflict`, and the page reloads.

A restart drops every instance. The page reconnects with a zero `T`. [Deployment](deployment.md#run-several-processes) covers load balancers and rolling deployments.

## Security

The instance ID is 16 random bytes. Whoever holds it can act on the tab's state, and nothing else authorizes access. Datapages keeps the ID out of cookies, browser storage and broker subjects. The inline script that receives it keeps it in a closure and removes itself, and the page goes out with `Cache-Control: no-store`. A `Content-Security-Policy` must allow `script-src 'unsafe-inline'` or provide a nonce through `datapages.WithCSPNonce`.

An instance belongs to the tab, not to the session, and a sign-out leaves it in place. Only on a page whose stream reads the session does the sign-out end the stream, which releases the instance and reloads the page. Do not keep in the state what the next visitor of that tab may not see, or store the user ID with it as [Security](security.md#bind-per-tab-state-to-a-session) shows.

## See also

- [`State[T]`](../SPECIFICATION.md#parameter-datapagesstatet): the declaration rules, the lifecycle, the instance ID and the limit.
- [Events](events.md): dispatching and handling the events that re-render each tab.
- [Content Security Policy](../SPECIFICATION.md#content-security-policy): the inline scripts and `WithCSPNonce`.
- [Security](security.md) and [SECURITY.md](../SECURITY.md): what the application and the deployment are responsible for.
- [`example/todolist`](../example/todolist/): state initialized in `StreamOpen` and read by event handlers.
