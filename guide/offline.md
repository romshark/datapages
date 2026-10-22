# Offline and shims

The offline module stores page snapshots and loading shims in a service-worker cache. A snapshot opens without a network connection. A shim appears immediately and is replaced by the live page.

The service worker comes from the module `github.com/romshark/datapages/modules/offline`. Handlers write its cache through the `pageCache datapages.PageCacheWriter` parameter:

- `Set` stores a snapshot, which the worker serves only while the browser is offline.
- `SetShim` stores a shim, which the worker serves at once, online too, and replaces with the live page.

Browsers run service workers only on HTTPS or localhost. On any other origin, cache writes reach no worker.

## Add a fallback page

Start with the page the worker shows for a URL that has no snapshot while the browser is offline:

```go title="app/app.go"
// PageOffline is /offline
type PageOffline struct{ App *App }

func (PageOffline) GET(
	r *http.Request,
) (body datapages.Component, err error) {
	return pageOffline(), nil
}
```

`PageOffline` renders with a zero session, and the worker shows 1 copy to every visitor. Keep its content independent of the signed-in user. The worker caches it when it installs. Without `PageOffline`, it shows a minimal built-in page.

## Configure the worker

List every file a snapshot needs in `Assets`, including the Datastar bundle. The worker caches them when it installs. It caches other files only when a page it controls loads them online, and the first page a visitor opens loads before the worker controls it:

```go title="app/offline.go"
const offlineWorkerVersion = 1

func OfflineConfig() offline.Config {
	return offline.Config{
		WorkerVersion: offlineWorkerVersion,
		Assets: []string{
			assets.Path("style.css"),
			assets.Path("datastar-1.0.4.js"),
		},
	}
}
```

The sample serves the bundle from the app's assets, as [Self-host the Datastar bundle](assets.md#self-host-the-datastar-bundle) shows. Put the library version in the file name, such as `datastar-1.0.4.js`. Browsers and the worker cache files by URL. If an upgrade keeps the name, they can keep using the old file from their cache. A new name is a new URL, which they download.

The other fields of `offline.Config`:

| Field | Default | Use |
| ----- | ------- | --- |
| `WorkerVersion` | 1 | identifies the installed worker and its cache |
| `Assets` | none | files the worker caches when it installs |
| `ScriptURL` | `/service-worker.js` | path of the worker script; the worker controls the whole origin either way |
| `OfflineClass` | `is-offline` | class set on `<html>` while the browser is offline |
| `CrossOriginDestinations` | `image`, `style`, `script`, `font` | request destinations cached from other origins; an empty non-nil slice caches none |
| `ExcludePaths` | none | same-origin path prefixes the worker never caches |
| `CSPNonce` | the function of `WithCSPNonce` | nonce of the connectivity and registration scripts |

## Add the server option

Declaring `PageOffline` makes `datapages gen` generate the option `datapagesgen.WithOffline`, which passes the route of `PageOffline` to the module. Add it in `cmd/server/main.go` with the bundle URL:

```go title="cmd/server/main.go"
datapages.WithDatastarJS(assets.Path("datastar-1.0.4.js")),
datapagesgen.WithOffline(app.OfflineConfig()),
```

The option serves the worker script and adds a connectivity script to every HTML response. It adds the registration script to the responses for a browser whose worker is missing or older than `WorkerVersion`. No template registers the worker.

Response compression runs after `WithOffline`, which therefore edits uncompressed HTML. Put a custom compressing middleware before `WithOffline` in the options, or its responses get no registration. With `WithCSPNonce`, the function must return the same nonce for every call with 1 request. A page served from the cache of the worker carries no nonce. See [Service Worker](../SPECIFICATION.md#service-worker).

## Write snapshots

Add `pageCache datapages.PageCacheWriter` to a `GET` or an action. Store an offline body with `Set`, using a URL from the generated `href` package:

```go title="app/app.go"
func (p PageItem) GET(
	r *http.Request,
	pageCache datapages.PageCacheWriter,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) (body datapages.Component, err error) {
	item, err := p.App.Item(r.Context(), path.Values.ID)
	if err != nil {
		return nil, err
	}
	if pageCache.Version() != item.Revision {
		pageCache.Set(href.PageItem(item.ID), itemOffline(item), item.Revision)
	}
	return itemPage(item), nil
}
```

`Version()` returns the version the browser holds for the URL of the current request, or 0 when it has no entry. The check keeps an unchanged snapshot from being sent again on every visit. A snapshot need not match the live body. Leave out what cannot work without the network, such as search and forms.

`Clear` removes 1 entry, and `ClearAll` empties the cache. The writes of a request take effect together after the handler returns without error, `ClearAll` first. A `GET` embeds them in the page. An action sends them over its SSE stream or in its response, depending on its signature. See [`pageCache`](../SPECIFICATION.md#parameter-pagecache-datapagespagecachewriter) for the delivery of each signature.

An action can `Set` other URLs, such as the pages of the tickets a purchase creates. `Version()` covers only the URL of the current request, which makes a `Set` for another URL unconditional.

While the browser is offline, the connectivity script sets the class `is-offline` on `<html>`. It also appends a `div` with the ID `datapages-offline-banner` and the text "You are offline, showing saved content." to the body. Style both in CSS, for example `.is-offline [data-needs-network] { opacity: .5; pointer-events: none }` for controls that need the network.

## Keep snapshots safe and current

- Pass a page body, not a complete document. Datapages wraps it in the same document as a live page, with the Datastar bundle and the tags of `App.Head` rendered for a zero session, and without the CSRF script.
- A cached page is only as complete as its assets. List the files it needs in `Assets`.
- Include every value that changes the snapshot in its version. A constant version caches once and never refreshes. Use `!=`, not `<`, for hashes and other unordered versions.
- Use versions above 0. `Version()` returns 0 for a URL without an entry, and `pageCache.Version() != 0` is false for it.
- Update or clear a URL when an action changes its data. A page cached by its own `GET` updates on its next online visit.
- Increment `WorkerVersion` after a Datapages upgrade or a change to `Assets`, `ExcludePaths`, `CrossOriginDestinations`, `OfflineClass` or `PageOffline`. The browser then installs the new worker and deletes the caches of earlier versions. Give a changed cross-origin asset a new URL or increment the version.

### Clear the cache at sign-in and sign-out

The browser stores snapshots for the origin, not for the session. Any script on the origin can read them, and they stay readable after the visitor signs out. Call `ClearAll` in the actions that sign a visitor in and out:

```go title="app/app.go"
// POSTSignOut is /sign-out/{$}
func (*App) POSTSignOut(
	r *http.Request,
	session Session,
	pageCache datapages.PageCacheWriter,
) (
	closeSession datapages.CloseSession,
	redirect datapages.Redirect,
	err error,
) {
	pageCache.ClearAll()
	return true, datapages.Redirect{URL: href.PageIndex()}, nil
}
```

Call it through a Datastar action, such as `data-on:click={ action.App.SignOut.POST() }`. Its response is a script that passes the writes to the worker and navigates once the worker applied them, or after 500 ms. An action that takes `pageCache` answers 406 to a plain form submission, and a `GET` that redirects drops the writes.

`ClearAll` deletes every entry of the cache, also the copy of `PageOffline` and the files of `Assets`. Same-origin assets return on their next online load. Until the worker installs again, an uncached URL opens the built-in page instead of `PageOffline` while offline.

Leave out of snapshots what must not outlive the session, such as payment details, tokens and the data of other people. See [Sessions and auth](sessions.md) for sign-in and sign-out and [Security](security.md) for what the browser keeps.

## Show a cached shim while a page loads

Use `SetShim` when a live page is slow. The worker shows the cached body while it fetches the live page, then replaces the document head and body:

```go title="app/app.go"
const shimVersion = 1

func (p PageIndex) GET(
	r *http.Request,
	pageCache datapages.PageCacheWriter,
) (body datapages.Component, err error) {
	rows, err := p.App.Rows(r.Context())
	if err != nil {
		return nil, err
	}
	if pageCache.Version() != shimVersion {
		pageCache.SetShim(href.PageIndex(), indexShim(len(rows)), shimVersion)
	}
	return indexPage(rows), nil
}
```

The first visit stores the shim. Later visits show it before the live response arrives. Increment `shimVersion` when its markup or data changes. This version is independent of `offline.Config.WorkerVersion`.

A shim stays visible after a failed fetch, also while the browser is offline. Use neutral placeholder text and claim nothing that is true only offline. The document of a shim has no session and no CSRF script, and the shim must contain no Datastar attributes. Keep actions and visitor-specific content in the live body.

If the application has no `PageOffline`, `datapages gen` generates no `WithOffline`. Install the module directly:

```go title="cmd/server/main.go"
offline.WithServiceWorker("", offline.Config{
	WorkerVersion: 1,
})
```

[`example/fast-shim`](../example/fast-shim/) compares pages with and without a shim.

## Test it locally

`datapages watch` serves `http://localhost:7331`, which is a secure context. Another device that opens the server by its network address over plain HTTP gets no worker. Serve HTTPS to test there.

1. Open a page of the app. Its response registers the worker, which then controls every page of the origin.
2. Open the pages whose handlers call `Set`.
3. Switch the browser offline, for example with the Offline checkbox under Application > Service workers in Chrome DevTools.
4. Navigate. A URL with a snapshot or a shim shows it, and any other URL shows `PageOffline`.

Stopping the server also makes the worker serve snapshots. The browser then stays online, and neither the `is-offline` class nor the banner appears.

> [!WARNING]
> The worker stays registered for its origin, which is the scheme, the host and the port, after the server stops. It also answers for the next application served at that origin, such as another project run with `datapages watch`, with the shims and assets it cached.

A hard reload, such as Shift+Cmd+R or Ctrl+Shift+R, bypasses the worker for 1 navigation. To remove the worker, unregister it in Chrome DevTools under Application > Service workers, or run this in the console of a page of the origin:

```js
navigator.serviceWorker.getRegistrations()
  .then((rs) => Promise.all(rs.map((r) => r.unregister())));
caches.keys().then((ks) => Promise.all(ks.map((k) => caches.delete(k))));
```

Unregistering keeps the caches, which the second statement deletes. The next page of an application with the offline module registers the worker again.

## See also

- [`pageCache`](../SPECIFICATION.md#parameter-pagecache-datapagespagecachewriter): the methods, the delivery rules and the action examples.
- [Service Worker](../SPECIFICATION.md#service-worker): installation, updates, caching rules and the CSP of cached pages.
- [`example/offline-cache`](../example/offline-cache/): a ticketing app with snapshots, `PageOffline` and sign-out.
- [Static assets](assets.md): serving the files that offline pages need.
- [Sessions and auth](sessions.md) and [Security](security.md): sign-out and what a snapshot must not contain.
- [`offline.Config`](https://pkg.go.dev/github.com/romshark/datapages/modules/offline#Config): the configuration fields.
