# Troubleshooting

Find the observed error or behavior below. Each section gives its cause and the command or code change that fixes it.

## `datapages gen` reports an undefined `href` or `action` helper

```text
at app_templ.go:39:80: undefined: action.PageIndex

error: parsing app package: 1 error(s)
```

A template calls a helper of a page or action that `datapages gen` has not generated yet, and `templ generate` already compiled the call into `app_templ.go`. `datapages gen` type-checks the app package with its `*_templ.go` files and refuses a package that does not compile, which keeps it from writing the helper the template needs.

Fix it in 3 steps:

1. Remove the call from the `.templ` file. When it was the last use of the `action` or `href` import of that file, remove the import too, or Go reports `imported and not used`.
2. Run `templ generate`, then `datapages gen`, which writes the helper.
3. Restore the call and the import, and run `templ generate` again.

To avoid it, add the declaration in Go and run `datapages gen` before a template calls its helper. See [The build loop](application-model.md#the-build-loop).

## Adding a field to a query struct stops `datapages gen`

```text
at app_templ.go:113:100: cannot convert q (variable of struct type SearchQuery) to type href.QueryPageSearch
```

A template converts a named query type of the app package to the query type of the `href` package, as `href.PageSearch(href.QueryPageSearch(q))`. Go converts between struct types only when their fields match. The new field in `SearchQuery` makes the conversion fail against the `href` type that the last run generated, and `datapages gen` refuses the package before it can generate the new `href` type. `datapages watch` stops regenerating for the same reason.

Remove the conversion for 1 generation:

1. Replace `href.QueryPageSearch(q)` with `href.QueryPageSearch{}` in the `.templ` file, which compiles against the old and the new type.
2. Run `templ generate` and `datapages gen`, or wait for `datapages watch` to do both.
3. Restore `href.QueryPageSearch(q)` and run `templ generate`.

Commenting out the element with `<!-- -->` works too, as long as another call keeps the `href` import of the file in use.

## `datapages gen` cannot import templ

```text
at app_templ.go:8:8: could not import github.com/a-h/templ (invalid package name: "")
```

`go.sum` lacks the entries of a dependency, as after copying a project or editing `go.mod` by hand. Run `go mod tidy`. `datapages gen` runs it at its end, which is why a second `datapages gen` succeeds.

## `go.mod` requires a newer Datapages than the CLI

```text
error: go.mod requires github.com/romshark/datapages v0.12.0 but you are running v0.11.0
  run: go install github.com/romshark/datapages/cmd/datapages@v0.12.0
```

A newer CLI raised the requirement in `go.mod`, as `datapages gen` does when the CLI is newer than the module. An older CLI could misread the module and refuses to run. Install the version the message names. See [Upgrading](upgrading.md).

## `datapages gen` says the agent instructions differ

```text
Agent instructions differ from this CLI; run `datapages init -n` to update them (edits are backed up).
```

The skills in `.claude/skills` or `.agents/skills` were written by another release of the CLI. The command succeeded. Run `datapages init -n` to replace them. It keeps an edited file as a `.bak` copy. See [AI agents](ai-agents.md).

## The dev server serves blank pages

The dev server answers `200`, but the document ends after `<body ...>` and the server logs an error such as this one, with the paths shortened:

```text
{"level":"ERROR","msg":"rendering PageIndex","err":"templ: failed to get watched string: templ: failed to get watched strings for \".../app/app_templ.go\": templ: failed to open .../templ_<hash>.txt: open .../templ_<hash>.txt: no such file or directory"}
```

In `datapages watch`, the app renders the text of its templates from files that templ watch mode writes into the system temporary directory. Every `templ generate` deletes those files when it exits, also when you run it by hand or through a `make` or mage target while the watcher runs.

Restart `datapages watch`, which writes the files again. Saving a `.templ` file with unchanged content does not help, because the watcher ignores a change that leaves the content of the file the same. A real edit of a `.templ` file restores the text of that file only.

Do not run `templ generate` while `datapages watch` runs on the module. A script can check the [watch lock](cli.md#watch-lock) first.

## The page does not reload after a change

The browser shows the address of the app, such as `http://localhost:8080`, which the app logs as `"msg":"listening HTTP","addr":"127.0.0.1:8080"`. Pages loaded there bypass the dev server. They do not reload after a rebuild and show no build errors.

Open the address of the dev server, `http://localhost:7331` unless `--host` sets another. `datapages watch` prints it only with `watch.log.level` set to `verbose` or `debug`. See [`datapages watch`](cli.md#datapages-watch) and [`level`](configuration.md#level).

## Changes to `.env` have no effect under `datapages watch`

The exclude pattern `.*` of the scaffold's `datapages.yaml` also matches `.env`. A change to `.env` therefore restarts nothing, and the app keeps the values it read at its last start. Restart `datapages watch` after editing `.env`. See [`exclude`](configuration.md#exclude).

## `datapages watch` cannot build the app

The dev server shows a `go` error such as:

```text
stat /home/me/shop/app/cmd/server: directory not found
```

Start `datapages watch` in the module root, as [`datapages watch`](cli.md#datapages-watch) describes.

## Go changes do not regenerate and dev-mode assets answer 404

[`dir-work`](configuration.md#dir-work) is set in `datapages.yaml`. The watcher matches the paths of changed files against the app package directories relative to that directory, which then match nothing, and the app reads its assets in dev mode from `./app/static` relative to it, which does not exist. Remove `dir-work`, or read the configuration page for what else it changes.

## `datapages watch` asks for `--app`

```text
error: this module builds 2 applications (app/admin, app/frontend), pass the directory of the one to run with --app
```

The module has more than 1 app package, and 1 dev server runs 1 of them. Pass the directory of the app package, as `datapages watch --app app/admin`. See [Multiple applications](multiple-applications.md#run-1-application-in-development).

## The address is already in use

`datapages watch` stops with:

```text
error: localhost:7331 is already in use (another `datapages watch` running?)
```

Another process listens on the address of the dev server, often a `datapages watch` of another project. Stop it, or pass another address, as `datapages watch --host localhost:7332`.

The app itself can fail the same way. Its log then holds:

```text
listening on localhost:8080: listen tcp 127.0.0.1:8080: bind: address already in use
```

The `cmd/server/main.go` of the scaffold listens on `HOST` and `PORT`, `localhost:8080` by default, and reads them from the environment or from `.env`. Stop the other process or set another port, as `PORT=8090` in `.env`. Under `datapages watch`, also set [`app-host`](configuration.md#app-host) in `datapages.yaml` to the new address, as `http://localhost:8090`, because the dev server forwards there. Then restart `datapages watch`.

## The Toxiproxy port is in use

`datapages watch` stops with:

```text
error: listening on watch.toxiproxy.host: listen tcp 127.0.0.1:7332: bind: address already in use; set it to a free port or set watch.toxiproxy.disabled
```

Another process listens on the address that [`watch.toxiproxy.host`](configuration.md#host) sets, often a `datapages watch` of another project. The same error names `watch.toxiproxy.ui-host` for the settings page. Stop the other process, set a free port, or remove `host` and `ui-host` to let the OS pick the ports. Set [`disabled: true`](configuration.md#disabled) to run without the proxy.

A port of `host` or `ui-host` that equals the port of `--host` or `app-host` stops `watch` before it listens:

```text
error: watch.toxiproxy.host localhost:7331 uses the port of --host; set it to a free port
```

Both errors stop `watch` only when `datapages.yaml` has a `watch.toxiproxy` section, as the scaffold has. Without the section, `watch` prints the cause after `warning: toxiproxy is off:` and runs without the proxy. See [Slow and failing networks](network.md#turn-the-proxy-off).

## Requests through the Toxiproxy proxy fail or hang

The [preset](network.md#apply-a-preset) in effect applies toxics that end or delay requests, such as Unreachable, `timeout` or `reset-peer`. Pick No throttling in the settings page, or open the `--host` address of the dev server, which applies no toxics. Every start of `datapages watch` begins with No throttling.

## Another project on the same address shows wrong pages or styles

After an application with [offline support](offline.md) ran on `http://localhost:7331`, another project served there shows that application's pages, styles or scripts on a normal reload, while a hard reload, Shift+Reload, shows the right page.

The offline service worker registers for the whole origin at scope `/`. It stays registered after you switch projects. The worker can answer navigations with stored shims and same-origin asset requests with stored copies. A hard reload bypasses it for 1 load.

Unregister the worker in the browser developer tools. In Chrome, use Application, Service workers. Clearing the origin's site data also removes it. Give each project its own `--host` port to keep the workers on separate origins.

## The server exits at startup with a NATS or session key error

The `cmd/server/main.go` of the scaffold stops before it listens:

| Log line | Cause |
| --- | --- |
| `ERROR NATS_URL not set` | Neither the environment nor a `.env` file in the current directory sets `NATS_URL`. |
| `ERROR SESSION_ENCRYPTION_KEY not set` | The same for `SESSION_ENCRYPTION_KEY`. |
| `ERROR decoding SESSION_ENCRYPTION_KEY err="encoding/hex: invalid byte: U+0078 'x'"` | The key is not hexadecimal. |
| `ERROR opening NATS connection err="nats: no servers available for connection"` | No NATS server answers at `NATS_URL`. |
| `ERROR initializing session manager err="encryption key must be exactly 16 bytes"` | The key has not 32 hexadecimal digits. |

`datapages init` writes `.env` with `NATS_URL=nats://localhost:4222` and a random key, and `compose.yaml` with a NATS server. Run the server from the module root, where `.env` is, and start NATS with `make up`, which runs `docker compose up -d`. `datapages init` writes `.env` only when it sets up a project. To write it again, set `NATS_URL=nats://localhost:4222` and a new key, such as the output of `openssl rand -hex 16`. A new key ends the sessions encrypted with the old one.

A single-process application can drop NATS and use `modules/messaging/inmem` and `modules/sessions/inmem`. See [Deployment](deployment.md).

## A form submission answers 406 Not Acceptable

The action takes `datapages.Signals`, `datapages.SSE` or `datapages.PageCacheWriter` and answers only requests with `Datastar-Request: true`. A plain HTML form submission does not send it. Submit the form through a Datastar action:

```templ
<form data-on:submit__prevent={ action.PageLogin.Submit.POST() }>
```

To send the form fields instead of signals, add `action.WithContentType(action.ContentTypeForm)` and read them from the request in an action without `datapages.Signals`. See [Templates and forms](templates.md).

## An action answers 403 Forbidden

There are 2 causes:

- CSRF. A `POST`, `PUT`, `PATCH` or `DELETE` request that carries the session cookie must send `X-CSRF-Token`. The CSRF script of the page adds it to Datastar requests. A plain HTML form does not send it, and a page rendered before the visitor signed in carries no script for the new session. Submit forms through Datastar actions, and navigate to a new page after sign-in, as a `datapages.Redirect` does.
- Origin. An action that takes neither signals nor SSE accepts plain forms and refuses a request whose `Sec-Fetch-Site` is `same-site` or `cross-site`, or whose `Origin` differs from `Host`. Serve the page and the action from 1 origin. A proxy must pass the original `Host` header.

See [Sessions and auth](sessions.md) and [Plain Forms and CSRF](../SPECIFICATION.md#plain-forms-and-csrf).

## A stateful action answers 409 Conflict

The answer carries `Datapages-Retry: reconnect`, and the page reloads once. The action takes `datapages.State[T]`, and the server holds no state for the `Datapages-Instance` of the tab. The state exists only while the stream of the tab is connected to the same server process:

- The tab was hidden, which closed its stream and released its state.
- The server restarted, and the stream has not reconnected yet.
- A load balancer sent the action to another server than the stream. Route the stream and the actions of a tab to 1 server by `Datapages-Instance`, by session or with an affinity cookie.
- The stream was refused with `503 Service Unavailable` because the server reached its limit of live states.

See [Per-tab state](state.md) and [Reconnects](request-lifecycle.md#reconnects).

## The page reloads by itself

Datapages reloads a page in 5 cases. The network panel of the developer tools shows which one:

| What the network panel shows | Cause |
| --- | --- |
| no failed request, the tab was in the background | A page with a stream reloads when its tab becomes visible again. Return [`disableRefreshAfterHidden`](../SPECIFICATION.md#get-return-value-disablerefreshafterhidden-datapagesdisablerefreshafterhidden) or [`enableBackgroundStreaming`](../SPECIFICATION.md#get-return-value-enablebackgroundstreaming-datapagesenablebackgroundstreaming) from `GET` to change it. |
| `205 Reset Content` with a `Datapages-Build` header | A server of another build answered. Open pages reload after every deployment. During a rolling deployment, route each client to 1 build. |
| a stream event that appends a script with `location.reload()` | The session of the stream closed or expired, as after a sign-out in another tab. |
| `409 Conflict` with `Datapages-Retry: reconnect` | A stateful action found no state. See the previous section. |
| nothing, after Back or Forward | A page with per-tab state reloads when the browser restores it from its back/forward cache. |

A page that reloads in a loop on a rolling deployment alternates between builds. See [Deployment](deployment.md) and [Request lifecycle](request-lifecycle.md#a-new-deployment).

## The browser console reports a Content Security Policy violation

Chrome reports:

```text
Executing inline script violates the following Content Security Policy directive 'script-src 'self' https://cdn.jsdelivr.net'. Either the 'unsafe-inline' keyword, a hash ('sha256-...'), or a nonce ('nonce-...') is required to enable inline execution. The action has been blocked.
EvalError: Evaluating a string as JavaScript violates the following Content Security Policy directive because 'unsafe-eval' is not an allowed source of script: script-src 'self' https://cdn.jsdelivr.net".
```

Datapages writes inline scripts into its pages, for the build ID, the per-tab state ID and the CSRF token, and Datastar evaluates the expressions of the `data-*` attributes. A policy without `'unsafe-inline'` and `'unsafe-eval'` blocks them, and the page opens no stream and runs no action. Mint a nonce per request, write it into the policy, and give it to Datapages with `datapages.WithCSPNonce`:

```go title="cmd/server/main.go"
type nonceKey struct{}

func nonceOf(r *http.Request) string {
	n, _ := r.Context().Value(nonceKey{}).(string)
	return n
}

// withCSP mints 1 nonce per request and writes it into the policy.
func withCSP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		nonce := base64.StdEncoding.EncodeToString(b)
		w.Header().Set("Content-Security-Policy",
			"script-src 'nonce-"+nonce+"' 'strict-dynamic'")
		ctx := context.WithValue(r.Context(), nonceKey{}, nonce)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
```

```go title="cmd/server/main.go"
opts = append(opts,
	datapages.WithMiddleware(withCSP),
	datapages.WithCSPNonce(nonceOf),
)
```

Datapages then writes the nonce on each of its scripts and as `data-nonce` on `<html>`, which switches Datastar to evaluating expressions without `'unsafe-eval'`. See [Content Security Policy](../SPECIFICATION.md#content-security-policy) and [Security](security.md).

## An asset URL answers 404

- The URL spells a path element differently from the file, as `/static/App.js` for `app.js`. Datapages compares the spelling byte for byte on every system, also on file systems that ignore case. Write asset URLs with `href.Asset` or `assets.Path` and spell the file name exactly.
- The URL names a directory, as `/static/`. `datapages.WithAssets(app.StaticFS, false)` answers 404 instead of listing it.
- `cmd/server/main.go` does not pass `datapages.WithAssets`. `datapages gen` writes the option only into a `main.go` it creates.
- In dev mode, the app reads assets from `./app/static` relative to its working directory. See [Go changes do not regenerate and dev-mode assets answer 404](#go-changes-do-not-regenerate-and-dev-mode-assets-answer-404).

See [Static assets](assets.md) and [Asset Paths](../SPECIFICATION.md#asset-paths).

## Browsers keep an old asset after a deploy

The server sends a positive `MaxAge` as `Cache-Control: public, max-age=3600`. A browser then reuses its copy without asking until that time passes. Set `MaxAge` only for files whose names change with their content, such as `datastar-1.0.4.js`. Use the zero `AssetsCacheConfig` for other files. It sends `max-age=0` and an `ETag`.

The offline service worker has a separate cache. It keeps a same-origin asset until its `max-age` passes and never updates a cross-origin asset. See [Static assets](assets.md) and [Changing Assets](../SPECIFICATION.md#changing-assets).

## Events do not reach pages served by another process

`modules/messaging/inmem` delivers events within 1 process. With 2 or more server processes, a dispatch reaches only the streams of the process that dispatched it. Connect every process to 1 NATS server with `modules/messaging/natscore`. See [Deployment](deployment.md).

## Every user is signed out after a restart

`modules/sessions/inmem` keeps sessions in the memory of the process, which a restart empties. Use a store that persists them, such as `modules/sessions/natskv`. See [Sessions and auth](sessions.md).

## See also

- [Request lifecycle](request-lifecycle.md): the headers and status codes behind the answers on this page.
- [Application model](application-model.md): how `datapages gen` reads the app package and reports errors.
- [CLI](cli.md): the commands, their exit status and the watch lock.
- [datapages.yaml](configuration.md): the settings of `datapages watch`.
- [Slow and failing networks](network.md): the Toxiproxy proxy of `datapages watch`.
- [Deployments](../SPECIFICATION.md#deployments) and [Dev Mode](../SPECIFICATION.md#dev-mode): the exact rules for reloads and dev mode.
