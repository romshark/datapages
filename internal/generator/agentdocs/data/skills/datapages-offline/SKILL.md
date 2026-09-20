---
name: datapages-offline
description: >-
  Serve a Datapages app offline: the offline module and its service worker,
  the PageOffline fallback, and writing page snapshots from handlers through
  the pageCache parameter.
---

# Offline support

Read `datapages` first for the build loop, hard rules and naming conventions.

`modules/offline` registers a service worker that serves cached page snapshots while the browser is offline. Handlers decide what is cached through the `pageCache datapages.PageCacheWriter` parameter. Without it nothing is cached and the worker only serves the fallback page.

## Wire the module

```go
opts := []datapages.ServerOption{
	// Self-host Datastar: the CDN is unreachable offline.
	datapages.WithDatastarJS(assets.Path("datastar.js")),
	// Generated because the app declares PageOffline. It passes that page's
	// route to the worker, so the route stays declared on the page type only.
	datapagesgen.WithOffline(app.OfflineConfig()),
}
```

```go
// OfflineWorkerVersion is the worker's own version. Bump it whenever the
// worker script or the precached asset set changes: the browser then installs
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

```go
// PageOffline is /offline
type PageOffline struct{ App *App }

func (PageOffline) GET(r *http.Request) (body datapages.Component, err error) {
	return pageOffline(), nil
}
```

`PageOffline` is the fallback for a URL that is not in the cache. It always renders with a zero `Session`. Datapages uses a minimal built-in page when the app declares none.

While the browser is offline the module toggles a class on `<html>`, so offline state is styled without any Go code. It defaults to `is-offline` and is configurable through `Config.OfflineClass`.

```css
.is-offline [data-needs-network] { opacity: .5; pointer-events: none }
```

## Write the cache from a handler

`pageCache datapages.PageCacheWriter` is an optional parameter of a `GET` and of an action, in any position.

```go
func (p PagePost) GET(
	r *http.Request,
	session Session,
	pageCache datapages.PageCacheWriter,
	path datapages.Path[struct {
		Slug string `path:"slug"`
	}],
) (body datapages.Component, err error) {
	post, ok := p.App.Post(path.Values.Slug)
	if !ok {
		return nil, datapages.ErrNotFound
	}
	view := postPage(post)
	if ver := postVersion(post); pageCache.Version() != ver {
		pageCache.Set(href.PagePost(post.Slug), postOffline(post), ver)
	}
	return view, nil
}
```

| method | effect |
| ------ | ------ |
| `Version()` | the version the client holds for **this request's URL**, 0 if none |
| `Set(url, body, version)` | cache `body` under `url` at `version` |
| `Clear(url)` | drop one entry |
| `ClearAll()` | drop the whole cache |

`Version()` cannot report the version held for any other URL, so caching another page is always unconditional.

## Rules

- **Pass the page body, not a document.** Datapages wraps a cached entry in the same document shell as a live page (`<head>`, stylesheets, Datastar bundle). Hand-rolling `<!DOCTYPE html>` around the body nests one document inside another.
- **A cached page is only as complete as its assets.** Caching the HTML is not enough: its stylesheets, scripts, fonts and images must be cached too. The app shell goes in `Config.Assets` and is precached when the worker installs. Everything else is cached the first time it loads online: same-origin files other than what Datastar requests, and cross-origin requests whose destination is listed in `Config.CrossOriginDestinations` (stylesheets, scripts, fonts and images by default). An asset that is neither listed nor ever loaded online is missing offline.
- **Version by everything the snapshot depends on.** A constant version caches once and never refreshes. Include the content state, such as an item count or an ownership flag.

  ```go
  // The body renders a different call-to-action depending on ownership, so the
  // version accounts for it. A constant would freeze the first snapshot.
  ver := snapshotVersion(session.UserID, owned) // e.g. an FNV-1a hash of both
  if pageCache.Version() != ver {
  	pageCache.Set(href.PagePost(slug), postOffline(post), ver)
  }
  ```
- **Use `!=`, not `<`, for an unordered version key** such as a hash. `<` only re-caches on an increase, which silently keeps a stale snapshot.
- **When one change invalidates many pages, call `ClearAll()`.** Re-caching every affected URL from one handler is impractical or impossible. Nothing repopulates on its own: an entry comes back only when a handler `Set`s that URL again. A lazily cached page comes back on its next online visit, because after a clear `Version()` reports 0 and the version guard fires. This applies to anything that changes how pages render across the board, such as a locale or permission change. Signing in and out is the common case: a snapshot cached for a guest still shows the signed-out navigation after login, so call `ClearAll()` in both actions.
- **A cached page stays stale until something `Set`s it again.** A lazily cached page refreshes on its next online visit. When a state change elsewhere invalidates it, re-`Set` it from the action that caused the change.

## Caching strategies

- **Lazy**: a handler caches the page it renders. The copy exists from the first visit on and an unvisited page costs nothing. A news article is cached when it is read.
- **Eager**: a handler caches URLs other than the one it serves, so a page is available before it is ever opened. A ticket purchase action caches the ticket pages, which makes the ticket viewable offline.

## Delivery

Queued writes reach the worker differently depending on the handler. The generated code picks the delivery from the signature, first row that matches. An action on `*App` follows the same rules as one on a page.

| handler | delivery |
| ------- | -------- |
| `GET` | trailing `<script>` baked into the HTML response |
| action with `sse` | script flushed over the SSE stream |
| action returning `redirect`, no `sse` | JS in the `text/javascript` redirect response, posted **before** navigating |
| action returning only a body | trailing `<script>` baked into the rendered document |
| action returning neither | script flushed over an SSE stream opened for it, which only a Datastar request can read |

`newSession` and `closeSession` cannot be combined with an `sse` parameter. Sign-in and sign-out therefore take `pageCache` and return a `redirect`, and their queued writes are posted before the navigation runs.
