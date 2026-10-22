# Static assets

Datapages can compile stylesheets, scripts, fonts and images into the server binary. The generated packages build their URLs, and the server handles caching and compression.

## Declare the files

Put the files in a directory of the app package, such as `app/static/`. Declare an `embed.FS` variable whose doc comment names the URL path to serve them at:

```go title="app/app.go"
// StaticFS is /static/
//
//go:embed static/*
var StaticFS embed.FS
```

The doc comment sets the URL prefix the way the doc comment of a page sets its route. The `//go:embed` directive names the directory. `datapages gen` and `datapages lint` reject a declaration that breaks these rules:

- An app package declares at most 1 such variable.
- The prefix starts and ends with `/` and is not `/`. It contains only ASCII letters, digits, `-`, `.`, `_`, `~` and `/`.
- The directive names exactly 1 directory inside the app package. `static/*.css` names files, not a directory.
- No page or action route starts with the prefix, which reserves every path below it.

## Serve the files

Pass the variable to the server in `cmd/server/main.go`. `datapages init` and `datapages gen` write that file only when it is missing, and neither adds the option to an existing one:

```go title="cmd/server/main.go"
opts = append(opts, datapages.WithAssets(app.StaticFS, false))
```

`false` answers 404 for a directory without an `index.html` instead of listing its files. The generated server serves the `static` directory of the `embed.FS` below the prefix, as `app/static/style.css` at `/static/style.css`. Without the option, every path below the prefix receives the 404 page. `datapages.NewServer` returns an error for `WithAssets` when the app package declares no assets.

`datapages.WithAssetsFS` takes an `http.FileSystem` in place of the `embed.FS` and serves it below the declared prefix as it is, without extracting a subdirectory and also in dev mode.

## Set cache headers

Assets carry `Cache-Control: public, max-age=0` and an `ETag` computed from the file contents. The browser asks before every reuse, sends the `ETag` back in `If-None-Match` and receives `304 Not Modified` without a body while the file is unchanged. `datapages.WithAssetsCache` changes these headers.

`MaxAge` lets browsers reuse files without asking. It applies to every file below the prefix. Set it only when every file name changes with its content, as `datastar-1.0.4.js` or `app.3f9a1c.css` does. Otherwise browsers keep old files after a deploy until `MaxAge` ends. For such names, let browsers keep files for a year:

```go title="cmd/server/main.go"
datapages.WithAssetsCache(datapages.AssetsCacheConfig{
	MaxAge:    365 * 24 * time.Hour,
	Immutable: true,
}),
```

This sends `Cache-Control: public, max-age=31536000, immutable`. `Immutable` keeps a reload from revalidating a fresh file and requires a positive `MaxAge`. `CacheControl` sets the header value as written and excludes `MaxAge` and `Immutable`. A value with `no-transform` keeps the assets uncompressed. `DisableETag` omits the `ETag`, for a CDN or a compressing middleware that writes its own.

## Write asset URLs

Run `datapages gen` after declaring the variable and before any template calls the generated functions. It generates 2 functions that return the URL of a file:

- `assets.Path` in the package `app/datapagesgen/assets`
- `href.Asset` in the package `app/datapagesgen/href`

Both return `/static/style.css` for `"style.css"` and percent-encode the path. Use `href.Asset` in an `<a href>`, where `datapages lint` accepts only calls into the `href` package. Use either in other elements and in Go code:

```templ title="app/app.templ"
templ head() {
	<link rel="stylesheet" href={ assets.Path("style.css") }/>
	<script type="module" src={ assets.Path("app.js") }></script>
}

templ downloadLink() {
	<a href={ href.Asset("report.pdf") } download>Download the report</a>
}
```

`App.Head` adds tags to the `<head>` of every page:

```go title="app/app.go"
func (*App) Head(r *http.Request) datapages.Head {
	return head()
}
```

`datapages lint` checks the URLs of `<a href>` and of Datastar actions, not those of `<link>`, `<script>` or `<img>`. A hardcoded `/static/style.css` there passes the linter and breaks when the prefix changes.

Spell each path exactly as its file is named. A request reaches a file only when each element of its path matches the directory listing byte for byte. `/static/App.js` receives 404 for the file `app.js` on every system and in dev mode, also on a file system that ignores case, such as the default one of macOS. See [Asset Paths](../SPECIFICATION.md#asset-paths).

## Serve files that change

Outside dev mode, the server hashes a file on its first request. It keeps the hash, `ETag` and compressed copies until the process exits. Files passed through `WithAssets` cannot change because they are compiled into the binary.

Do not change files served from a directory through `WithAssetsFS` while the server runs unless asset compression and the `ETag` are both off. A changed file keeps its old `ETag`, and a compressed response can keep the old content until restart. See [Changing Assets](../SPECIFICATION.md#changing-assets).

Serve files that change at run time, such as uploads, through a GET action that returns `datapages.File`:

```go title="app/app.go"
// GETLogo is /logo.png
func (a *App) GETLogo(r *http.Request) (file datapages.File, err error) {
	f, err := a.Uploads.Open("logo.png")
	if err != nil {
		return datapages.File{}, datapages.ErrNotFound
	}
	info, err := f.Stat()
	if err != nil {
		return datapages.File{}, errors.Join(err, f.Close())
	}
	return datapages.File{Type: "image/png", Body: f, ModTime: info.ModTime()}, nil
}
```

`Uploads` is an `*os.Root` field of `App`. The response carries `Cache-Control: no-cache` and `Last-Modified`. The browser revalidates with `If-Modified-Since` and receives 304 while the file is unchanged. `href.App.Logo()` returns the URL. See [Actions](actions.md) for GET actions and [`datapages.File`](../SPECIFICATION.md#return-value-datapagesfile) for its fields.

## Self-host the Datastar bundle

Pages load the Datastar bundle from `https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.4/bundles/datastar.js` by default. Serve it from the app's assets when pages must not depend on another origin. Download it into the assets directory:

```sh
curl -fsSL -o app/static/datastar-1.0.4.js \
  https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.4/bundles/datastar.js
```

Pass its URL to the server:

```go title="cmd/server/main.go"
opts = append(opts, datapages.WithDatastarJS(assets.Path("datastar-1.0.4.js")))
```

Keep the version in the file name. Browsers and the offline service worker cache files by URL. If an upgrade keeps the name, they can keep using the old file from their cache. A new name is a new URL, which they download. `WithDatastarJS` accepts an `http`, `https` or relative URL. `WithCSPNonce` requires Datastar 1.0.3 or later. See [Security](security.md) for the `Content-Security-Policy`.

## Dev mode

Dev mode is on when `DATAPAGES_DEV_MODE` or `TEMPL_DEV_MODE` is not empty. `datapages watch` sets `TEMPL_DEV_MODE` before it starts the server. In dev mode the server:

- reads the files from the source directory, `./app/static` for the declaration above. The path is relative to the working directory of the server, which `datapages watch` sets to the module root. An edited, added or renamed file is served without a rebuild.
- sends `Cache-Control: no-cache` without `ETag` or `Last-Modified`. The browser downloads the file again on every use.
- ignores `WithAssetsCache`, `WithResponseCompression` and `WithAssetsCompression`, and sends assets and pages uncompressed.
- logs the warning `dev mode is on` at startup.

A production process that inherits either variable reads its assets from a directory it does not have and answers 404. The generated constant `assets.DevDir` holds the path. See [Dev Mode](../SPECIFICATION.md#dev-mode).

## Compression

Outside dev mode, Datapages compresses an eligible asset of at least 256 bytes on its first request. It keeps the copy in memory until the server exits. It uses brotli 6, or gzip 9 when the request does not accept brotli.

Eligible types include text, JavaScript, JSON, XML, YAML, TOML, SVG, WebAssembly, TrueType and OpenType fonts, icons and BMP images. PNG, JPEG, WOFF2 and other precompressed formats stay uncompressed. `datapages.WithNoAssetsCompression` turns compression off. [Performance](performance.md#asset-compression) explains precompression, other levels and disk storage.

## See also

- [Asset Paths](../SPECIFICATION.md#asset-paths), [Changing Assets](../SPECIFICATION.md#changing-assets) and [Dev Mode](../SPECIFICATION.md#dev-mode): the exact rules for paths, changed files and dev mode.
- [Asset Compression](../SPECIFICATION.md#asset-compression): the eligible types and the headers of compressed responses.
- [Performance](performance.md#asset-compression): precompression, compression levels and the disk store.
- [Offline and shims](offline.md): precache the assets that offline pages need.
- [Actions](actions.md): GET actions that serve files.
- [`WithAssets`](https://pkg.go.dev/github.com/romshark/datapages#WithAssets) and [`AssetsCacheConfig`](https://pkg.go.dev/github.com/romshark/datapages#AssetsCacheConfig): the Go API.
