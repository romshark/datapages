# fast-shim: Cached shims for slow pages

`app.SlowQuery` delays every page render by 900 ms to simulate a slow query. `/` and `/subpage` show a cached shim while the live page renders. `/noshim` and `/noshim2` wait for the live page.

| route | page | behavior |
| ----- | ---- | --------- |
| `/` | `PageIndex` | cached shim appears before the live page replaces it |
| `/subpage` | `PageSubpage` | cached shim appears before the live page replaces it |
| `/noshim` | `PageNoShim` | waits 900 ms for the live page on every visit |
| `/noshim2` | `PageNoShim2` | waits 900 ms for the live page on every visit |

On the first visit to a shimmed page, the cache is empty and the response takes 900 ms. On later visits, the service worker returns the cached shim and fetches the live page in parallel. Datastar replaces the `<head>` and `<body>` when the fetch completes.

https://github.com/user-attachments/assets/1be1f20c-815f-471e-8b5e-87137d4c5cd7

## Shim setup

`PageIndex.GET` in [`app/app.go`](./app/app.go) caches the shim:

```go
if pageCache.Version() != shimVersion {
	pageCache.SetShim(href.PageIndex(), shim(titleIndex, len(rows)), shimVersion)
}
```

`SetShim` stores a body that the worker may serve while online. The worker serves `Set` entries only while offline. The shim renders the layout with skeleton rows. It must not present offline-only state because the visitor sees it during an online navigation.

`Version()` reports the version stored for the current URL. The handler calls `SetShim` only when that version differs from `shimVersion`. Increment `shimVersion` after changing the placeholder markup.

`datapages gen` omits `datapagesgen.WithOffline` because the application has no `PageOffline`. [`cmd/server/main.go`](./cmd/server/main.go) installs the module directly with `offline.WithServiceWorker`. Increment `WorkerVersion` after a Datapages upgrade or a change to `Assets`, `ExcludePaths`, `CrossOriginDestinations`, or `OfflineClass`. The browser then reinstalls the worker and deletes its previous cache.

## Run

```sh
datapages watch
```

Open <http://localhost:8080>. Service workers require a secure context. Browsers treat localhost as secure; plain HTTP on other hosts is not secure.

Reload a shimmed page, or navigate away and back, to see the skeleton first. The Application tab in browser developer tools lists the worker and cache entries.

See [`datapages-offline`](./.claude/skills/datapages-offline/SKILL.md) for the page cache reference. [`example/offline-cache`](../offline-cache/) demonstrates `PageOffline` with the same module.
