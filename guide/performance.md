# Performance

Datapages compresses SSE streams, page and action responses, and static assets by default. The settings trade CPU time and memory for transfer size.

## SSE compression

Each stream holds an encoder from its first event until it closes. CPU and memory use therefore grow with the number of open streams. By default, a stream uses the first coding the browser accepts in the order zstd, brotli, gzip. The levels are zstd 1, brotli 4 with a 1 MiB window, and gzip 6. A preset changes the trade-off:

| Preset | Use | Sent per re-render | Heap per stream | Time |
| ------ | --- | ------------------ | --------------- | ---- |
| `datapages.SSECompressionBalanced()` | the default | 1.0% | 1.6 MiB | 1x |
| `datapages.SSECompressionLowMemory()` | many open streams with fragments below about 100 KB | 3.0% | 0.9 MiB | 0.9x |
| `datapages.SSECompressionLowBandwidth()` | few streams with slow or metered clients, brotli first | 0.06% | 3.5 MiB | 2.6x |
| `datapages.SSECompressionLargeFragments()` | fragments above about 1 MB, 4 MiB windows | 0.33% | 4.6 MiB | 1.6x |

The last 3 columns come from re-rendering a 100 KB list with 1 item changed. Apply a preset to every stream in `cmd/server/main.go`:

```go title="cmd/server/main.go"
datapages.WithSSECompression(datapages.SSECompressionLowMemory()),
```

Override it for the streams of 1 page with `StreamConfig`:

```go title="app/app.go"
func (PageReport) StreamConfig() datapages.StreamConfig {
	c := datapages.SSECompressionLargeFragments()
	return datapages.StreamConfig{Compression: &c}
}
```

Override it for the response of 1 action with a method named `StreamConfig` followed by the method name of the action:

```go title="app/app.go"
func (PageEditor) StreamConfigPOSTSave() datapages.StreamConfig {
	return datapages.StreamConfig{
		Compression: &datapages.SSECompressionConfig{Disabled: true},
	}
}
```

The method can also choose settings per request:

```go title="app/app.go"
func (PageFeed) StreamConfig(r *http.Request) (datapages.StreamConfig, error) {
	if r.Header.Get("Save-Data") != "on" {
		return datapages.StreamConfig{}, nil
	}
	c := datapages.SSECompressionLowBandwidth()
	return datapages.StreamConfig{Compression: &c}, nil
}
```

A non-nil `Compression` replaces the server setting, and its zero fields select the package defaults, not the values of the server. A nil `Compression` keeps the server setting. The `StreamConfig` of a page does not apply to its actions. Return a small set of configurations from a per-request method. The server shares encoders among at most 64 distinct configurations and gives every further one encoders of its own. See [StreamConfig](../SPECIFICATION.md#streamconfig) for the valid signatures and [Choose where compression runs](#choose-where-compression-runs) before disabling compression.

## Re-render whole fragments

An event handler can render a large fragment or the whole page again from current data, instead of working out which elements changed. Datastar morphs the result into the page. [`example/todolist`](../example/todolist/) sends its whole list this way. Compression keeps the cost low, because zstd and brotli encode the parts of a render that repeat an earlier render of the same stream as references to it. A 100 KB list with 1 item changed costs 1.0% of its size per re-render with the default, against 12.9% with gzip and its 32 KiB window.

This holds only while the previous render fits in the window. A fragment larger than its window cannot refer to its previous render and goes out at its full compressed size on every update. A 2 MB list sends 14% of its size per re-render with the default 1 MiB window, and 2% with the 4 MiB windows of `SSECompressionLargeFragments()`, which holds 3 times the memory per stream. With many open streams, keep fragments below the window or patch smaller elements.

## Response compression

Datapages compresses page and action bodies of at least 256 bytes, other than SSE streams. By default a body uses the first coding the browser accepts in the order zstd, brotli, gzip, at zstd level 3, brotli quality 5 or gzip level 6. A response holds an encoder only while it is written. A preset changes the trade-off:

- `datapages.ResponseCompressionLowCPU()` sets every level to 1 and prefers gzip over brotli. It holds 0.7 MiB per response instead of 1.8 MiB and sends 19.9% of an 8 KB page instead of 19.0%.
- `datapages.ResponseCompressionLowBandwidth()` prefers brotli for slow or metered clients. It sends 16.4% of an 8 KB page and takes 64 us instead of 14 us.

```go title="cmd/server/main.go"
datapages.WithResponseCompression(datapages.ResponseCompressionLowCPU()),
```

`datapages.File` responses stay uncompressed. Static assets use separate compression, which [Asset compression](#asset-compression) describes. To send precompressed content, serve an archive such as `export.csv.gz` with type `application/gzip`, or negotiate `Accept-Encoding` and set `Content-Encoding` in `Header`. Give each representation its own `ETag` and send `Vary: Accept-Encoding`.

The size of a compressed response can reveal a secret placed beside input an attacker controls. Send such a page with `Cache-Control: private, no-transform`, which a middleware can set, and Datapages leaves it uncompressed. Do the same for its `_$/` stream, or give the page a `StreamConfig` that returns `Compression: &datapages.SSECompressionConfig{Disabled: true}`. See [Security](security.md) and [Response Compression](../SPECIFICATION.md#response-compression).

## Asset compression

Datapages compresses eligible assets on their first request and keeps the copies in memory. The default, brotli 6, takes 1.5 to 4 ms for a file of about 220 KB. To compress them before the first request, create the compressor and precompress the embedded files:

```go title="cmd/server/main.go"
compressor, err := assetcompress.New(assetcompress.Config{})
if err != nil {
	return err
}
opts = append(opts, datapages.WithAssetsCompression(compressor))
go func() {
	if err := compressor.Precompress(ctx, app.StaticFS); err != nil {
		slog.Error("precompressing assets", slog.Any("err", err))
	}
}()
```

Pass the whole `embed.FS`. Copies are keyed by content, which lets them serve the subdirectory the server uses. Precompression stops at the first error, and requests compress the files it did not reach. Drop `go` to finish before serving, which delays startup.

### Choose compression levels

For copies 6% to 11% smaller, use brotli 11 and drop `go` before `Precompress`. Brotli 11 takes 45 to 95 times as long as brotli 6, and a request that finds no copy otherwise waits for its compression, up to 215 ms per 200 KB:

```go title="cmd/server/main.go"
compressor, err := assetcompress.New(assetcompress.Config{
	Levels: assetcompress.Levels{BrotliQuality: 11},
})
```

### Keep the copies on disk

Without a store, the copies stay in memory until the server exits, 1 per coding and each at about 10% to 40% of the size of its file. Persist them in a directory when they take too much memory, or when brotli 11 has megabytes to compress again after every restart, at 1 to 1.5 MB per second:

```go title="cmd/server/main.go"
store, err := disk.New("/var/cache/myapp/assets", disk.Config{})
if err != nil {
	return err
}
compressor, err := assetcompress.New(assetcompress.Config{Store: store})
```

The store serves each copy from its file instead of from memory. Skip it for a few assets at the default levels, which recompress in a few milliseconds after a restart. Give the store a directory that only the server can write to. `disk.New` creates a missing directory with mode 0700. On Unix it refuses a directory that every user can write to or that belongs to a user other than the server's and root. For a Kubernetes `emptyDir` volume, pass a subdirectory of its mount point. When a server starts, the store deletes the copies that no server opened for 7 days. Set `disk.Config.MaxUnused` to keep them longer.

### Use another store

To keep the copies elsewhere, such as in object storage that all servers share, implement [`assetcompress.Store`](https://pkg.go.dev/github.com/romshark/datapages/modules/assetcompress#Store) and pass it as `Store`. Every request for a stored copy calls `Open`. Cache remote copies locally. Keys contain the SHA-256 of the file, which lets the servers of every build share the store. The compressor never deletes copies. Expire old ones in the storage, for example with a lifecycle rule.

## Choose where compression runs

Disable compression for an entry point that only loopback clients reach, such as a desktop app whose webview loads from `127.0.0.1`. Compression there costs CPU and saves no transfer:

```go title="cmd/server/main.go"
datapages.WithNoResponseCompression(),
datapages.WithNoSSECompression(),
datapages.WithNoAssetsCompression(),
```

The coding depends only on the `Accept-Encoding` header of the request, not on the client address. Behind a reverse proxy, keep compression on unless a body filter such as nginx `sub_filter` must read the HTML. Then disable it, or set `proxy_set_header Accept-Encoding "";` in the proxy. Caches between the server and the browser must honor `Vary: Accept-Encoding`.

Compression runs after every middleware of `datapages.WithMiddleware`, which reads and edits uncompressed bodies. Dev mode sends pages, action bodies and assets uncompressed and keeps SSE compression. Measure outside dev mode.

## Other settings that affect performance

- Hidden tabs: by default a page closes its stream while its tab is hidden and reloads when the tab becomes visible again. A `GET` that returns `enableBackgroundStreaming` true keeps the stream and its encoder open while the tab is hidden and skips the reload. A `GET` that returns `disableRefreshAfterHidden` true only skips the reload. See [`enableBackgroundStreaming`](../SPECIFICATION.md#get-return-value-enablebackgroundstreaming-datapagesenablebackgroundstreaming).
- Per-tab state: each open stream of a page with `datapages.State[T]` holds 1 instance of `T`. By default a server holds at most `datapages.DefaultMaxConcurrentInstances`, 10,000, and answers further streams with 503. Size the limit by the memory of `T` with `datapages.WithStateConfig`. See [Per-tab state](state.md).
- Slow pages: a cached shim shows the layout at once while the live page renders. See [Show a cached shim while a page loads](offline.md#show-a-cached-shim-while-a-page-loads).
- Prefetching: `sse.Prefetch(urls...)` asks the browser to prefetch pages through the Speculation Rules API. Browsers without it ignore the request.
- Asset caching: the server answers unchanged assets with 304, and the `MaxAge` of `datapages.WithAssetsCache` saves the request for file names that change with their content. See [Set cache headers](assets.md#set-cache-headers).
- Templates: templ compiles templates to Go code. [FAQ](../FAQ.md#why-templ-instead-of-htmltemplate) compares its speed with `html/template`.

## Where the numbers come from

The numbers on this page come from benchmarks in the Datapages repository. The godoc of each configuration type lists their results:

- [`SSECompressionConfig`](https://pkg.go.dev/github.com/romshark/datapages#SSECompressionConfig): `BenchmarkReRender` in `runtime/sse` re-renders lists of 8 KB to 2 MB with 1 item changed.
- [`ResponseCompressionConfig`](https://pkg.go.dev/github.com/romshark/datapages#ResponseCompressionConfig): `BenchmarkResponseCompression` in `runtime/httpserve` compresses pages of 8 KB and 100 KB.
- [`assetcompress.Config`](https://pkg.go.dev/github.com/romshark/datapages/modules/assetcompress#Config): `BenchmarkPrecompress` in `modules/assetcompress` compresses files of the examples, such as a 222 KB script and a 218 KB stylesheet.

Times depend on the machine. Run a benchmark in a checkout of the repository to measure on yours. The sizes that `BenchmarkReRender` reports depend on the iteration count, because each iteration changes 1 more item of the list. `-benchtime 200x` reproduces the sizes of the godoc:

```sh
go test -run '^$' -bench BenchmarkReRender -benchtime 200x ./runtime/sse
```

## See also

- [Response Compression](../SPECIFICATION.md#response-compression) and [Asset Compression](../SPECIFICATION.md#asset-compression): the exact rules and the headers of compressed responses.
- [`sse datapages.SSE`](../SPECIFICATION.md#parameter-sse-datapagessse): the SSE compression defaults and options.
- [StreamConfig](../SPECIFICATION.md#streamconfig): the signatures of per-page and per-action settings.
- [Static assets](assets.md): cache headers, dev mode and the default asset compression.
- [Events](events.md): the events whose handlers re-render pages.
- [Security](security.md): compression next to secrets.
