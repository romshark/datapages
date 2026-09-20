---
name: datapages-offline
description: >-
  The Datapages service worker: offline page snapshots and cached shims
  written by handlers through the pageCache parameter, the offline module
  and its Config, and the PageOffline fallback.
---

# Offline and instant loads

Read `datapages` first for the build loop, hard rules and naming conventions.

`modules/offline` serves a service worker and injects its registration into pages. Handlers fill its cache through the `pageCache datapages.PageCacheWriter` parameter. One cache, two uses:

- **Snapshots** (`Set`): served only while the browser is offline.
- **Shims** (`SetShim`): served online too, at once, then replaced by the live page.

The worker runs only in a secure context (HTTPS or localhost). Everywhere else the API does nothing.

## Wire the module

Declaring `PageOffline` generates a `WithOffline` option carrying that page's route:

```go
// PageOffline is /offline
type PageOffline struct{ App *App }

func (PageOffline) GET(r *http.Request) (body datapages.Component, err error) {
	return pageOffline(), nil
}
```

```go
opts := []datapages.ServerOption{
	// Self-host Datastar: the CDN is unreachable offline.
	datapages.WithDatastarJS(assets.Path("datastar.js")),
	datapagesgen.WithOffline(app.OfflineConfig()),
}
```

```go
// OfflineWorkerVersion is the worker's own version. Bump it whenever the
// worker script or the precached asset set changes. The browser then installs
// the new worker and drops the caches of older versions.
const OfflineWorkerVersion = 1

func OfflineConfig() offline.Config {
	return offline.Config{
		WorkerVersion: OfflineWorkerVersion,
		// App shell precached on install so cached pages still render offline.
		Assets: []string{
			assets.Path("style.css"), assets.Path("datastar.js"),
		},
	}
}
```

Without `PageOffline` no option is generated. Wire the middleware directly, which is enough for shims:

```go
datapages.WithMiddleware(offline.Middleware("", offline.Config{WorkerVersion: 1}))
```

| `offline.Config` field | default |
| ---------------------- | ------- |
| `WorkerVersion` | 1; bump on a worker or shell change |
| `ScriptURL` | `/service-worker.js`; the scope is the whole origin either way |
| `Assets` | none; the shell precached on install |
| `OfflineClass` | `is-offline`, toggled on `<html>` while offline |
| `CrossOriginDestinations` | `image`, `style`, `script`, `font`; empty non-nil disables |

Style offline state in CSS, no Go code:

```css
.is-offline [data-needs-network] { opacity: .5; pointer-events: none }
```

## PageOffline

A reserved name, like `PageError404` and `PageError500`. The worker precaches it and serves it while offline for a URL it holds no entry for. It renders with a zero `Session`. Datapages uses a minimal built-in page when the app declares none.

## The pageCache parameter

Optional on a `GET` and on any action, page-level or on `*App`, in any position.

| method | effect |
| ------ | ------ |
| `Version()` | the version the client holds for **this request's URL**, 0 if none |
| `Set(url, body, version)` | cache `body` for `url`, served only while offline |
| `SetShim(url, body, version)` | cache `body` for `url`, served online too |
| `Clear(url)` | drop one entry |
| `ClearAll()` | drop the whole cache |

`url` comes from the generated `href` package. Writes are **deferred and atomic**: nothing reaches the client until the handler returns without error, then every call of that request applies together, `ClearAll` first.

`Version()` covers this URL only. Caching any other URL is therefore unconditional.

## Snapshots

```go
func (p PagePost) GET(
	r *http.Request,
	pageCache datapages.PageCacheWriter,
	path datapages.Path[struct {
		Slug string `path:"slug"`
	}],
) (body datapages.Component, err error) {
	post, ok := p.App.Post(path.Values.Slug)
	if !ok {
		return nil, datapages.ErrNotFound
	}
	if ver := postVersion(post); pageCache.Version() != ver {
		pageCache.Set(href.PagePost(post.Slug), postOffline(post), ver)
	}
	return postPage(post), nil
}
```

A snapshot need not match the live body. It is what the user sees with no network: leave out what cannot work there.

**Lazy**: a handler caches the page it renders. An unvisited page costs nothing. **Eager**: a handler caches URLs other than the one it serves, such as a purchase action caching the ticket pages it just created.

## Shims

A shim is a placeholder rendering of a page: its chrome with the slow parts replaced by skeletons. The worker paints it from cache at once and fetches the live page in parallel. Datapages emits the trigger that morphs the live page in. The shim itself carries no Datastar attributes.

```go
// shimVersion versions the cached shims. They hold no data. Only a code change
// bumps it.
const shimVersion = 1

func (p PageIndex) GET(
	r *http.Request, pageCache datapages.PageCacheWriter,
) (body datapages.Component, err error) {
	rows, err := p.App.Rows(r.Context())
	if err != nil {
		return nil, err
	}
	if pageCache.Version() != shimVersion {
		pageCache.SetShim(href.PageIndex(), shim(len(rows)), shimVersion)
	}
	return indexPage(rows), nil
}
```

A shim is shown online as well. It must not state anything that is only true offline. Offline the parallel fetch fails and the shim stays on screen.

## Rules

- **Pass the page body, not a document.** Datapages wraps a cached entry in the same document shell as a live page (`<head>`, stylesheets, Datastar bundle). Hand-rolling `<!DOCTYPE html>` around the body nests one document inside another.
- **A cached page is only as complete as its assets.** Its stylesheets, scripts, fonts and images must be cached too. `Config.Assets` is precached on install. Everything else is cached on its first load while online: same-origin files other than what Datastar requests (actions, hydrates and event streams always come from the network), and cross-origin requests whose destination is in `Config.CrossOriginDestinations`. An asset that is neither listed nor ever loaded online is missing offline.
- **Version by everything the body depends on.** A constant version caches once and never refreshes. Include the content state, such as an item count or an ownership flag.

  ```go
  // The body renders a different call-to-action depending on ownership, which
  // the version accounts for. A constant would freeze the first snapshot.
  ver := snapshotVersion(session.UserID, owned) // e.g. an FNV-1a hash of both
  if pageCache.Version() != ver {
  	pageCache.Set(href.PagePost(slug), postOffline(post), ver)
  }
  ```
- **Use `!=`, not `<`, for an unordered version key** such as a hash. `<` re-caches only on an increase and silently keeps a stale entry.
- **`ClearAll()` when one change invalidates many pages**, such as a locale or permission change. Re-caching every affected URL from one handler is impractical or impossible. Nothing repopulates on its own: an entry returns when a handler `Set`s that URL again. A lazily cached page returns on its next online visit, since after a clear `Version()` reports 0 and the version guard fires. Signing in and out is the common case: a snapshot cached for a guest still shows the signed-out navigation after login. Call `ClearAll()` in both actions.
- **An entry stays stale until something `Set`s it again.** A lazily cached page refreshes on its next online visit. When a change elsewhere invalidates it, re-`Set` it from the action that made the change.

## Delivery

The generated code picks how queued writes reach the worker from the handler signature, first row that matches. An action on `*App` follows the same rules as one on a page.

| handler | delivery |
| ------- | -------- |
| `GET` | baked into the page's HTML, applied on load, no extra request |
| action taking `sse` | sent over that stream |
| action returning `redirect` | carried in its `text/javascript` response, applied **before** the navigation; chosen even when the action also returns a body |
| action returning only a body | baked into the document it renders |
| action returning neither | sent over an SSE stream opened for that purpose, readable only by a Datastar request |

`newSession` and `closeSession` cannot be combined with `sse`. Sign-in and sign-out therefore take `pageCache` and return a `redirect`, which applies their writes before the navigation runs.
