# Changelog

This file records the notable changes of each release in the format of
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The release
workflow publishes the section of a version as its GitHub release notes.
Releases up to v0.10.0 have their notes on
[GitHub Releases](https://github.com/romshark/datapages/releases) only.

## [Unreleased]

### Added

#### `datapages lint` and `datapages gen`

- Accept `QUERYXXX` actions for the `QUERY` method of RFC 10008, which Datastar 1.0.4 sends with `@query`. Declare one on a page or on `App`, as in `// QUERYSearch is /search`, and build its expression with the `action` package, as in `action.PageIndex.Search.QUERY()`. Datastar sends the signals in the request body.
- Accept static and request-scoped `StreamConfig` methods on pages with SSE streams. The static form returns `datapages.StreamConfig` during `NewServer`. The request-scoped form returns `(datapages.StreamConfig, error)` per stream and may take the session, path and signals. Both override `datapages.WithSSECompression`. Invalid configurations fail `NewServer` or the stream request. A page or `App` configures the SSE response of an action the same way, with a method named `StreamConfig` plus the action's method name, such as `StreamConfigPOSTSave`.
- Serve files from generated handlers with `datapages.File`. Any action may return one; a read-only `GETXXX` action also gets a generated `href` builder. Applications previously needed custom middleware or a separate `net/http` handler and had to maintain its route and URL by hand.

#### Generated code

Run `datapages gen` to apply these.

- Add `datapages.WithSSECompression` to configure SSE compression and `datapages.WithNoSSECompression` to disable it. The presets `datapages.SSECompressionBalanced`, `datapages.SSECompressionLowMemory`, `datapages.SSECompressionLowBandwidth` and `datapages.SSECompressionLargeFragments` cover common trade-offs. A `Cache-Control: no-transform` set before a stream opens keeps it uncompressed.

#### Runtime and modules

- Reload open pages when a deployment changes the build. Set `datapages.WithBuildID` when replicas run different binaries of 1 release. A `Content-Security-Policy` must allow the inline build script.

#### `datapages init`

- Scaffold Web Components instead of requiring manual frontend setup. Choose Datastar Rocket, esbuild-bundled TypeScript, JavaScript modules or none interactively or with `--web-components rocket|typescript|javascript|none`; `-n` defaults to none. Each setup adds an attribute-and-event example. Rocket loads the matching CDN bundle. TypeScript requires Node.js; init installs and builds it, and `datapages watch` rebuilds changes.

### Changed

#### `datapages lint` and `datapages gen`

- Reject actions, `OnXXX` handlers, `StreamOpen`, `StreamClose` and `State[T]` on a page whose route ends in `{$}`, as `// PageUser is /user/{name}/{$}`. A `{$}` at the end of a page route declares a page that serves only `GET`. An action below such a route got an error saying that it was not under its page, with a suggested route that net/http rejects. Remove `{$}` from the route of such a page: it keeps its URL.
- Read an exported method on `App` named `QUERY` followed by an uppercase letter, such as `QUERYStats`, as an action. Such a method is rejected when it's a helper rather than an action: rename it.
- Read an exported method on `App` named `GET` followed by an uppercase letter, such as `GETStats`, as a GET action. Such a method is rejected when it's a helper rather than an action: rename it.
- Reject page and action routes below the static-files URL prefix. `net/http` treats them as more specific, so matching asset requests reach the handler instead of the file server. Move the route outside the prefix.
- Reject application code that imports a package under `github.com/romshark/datapages/runtime`, test files included. Those packages are for the generated code only, and any release may change them without notice. Use the `datapages` package or the generated packages instead.
- Skip type-checking the function bodies of the standard library and of downloaded modules when loading the app package. `datapages lint` on `example/classifieds` takes 277ms instead of 406ms, and `datapages watch` regenerates sooner after each change to a Go file.
- Reject an `OnXXX` handler that is not named after its event, as `OnNewMessage(event EventMessageSent, sse datapages.SSE)`. A page's `OnB` taking `EventA` hid an embedded `OnB` taking `EventB`, and the generated code did not compile. Rename the handler to `On` and the event name after `Event`, as `OnMessageSent`.
- Require the app package directory in `datapages watch --app`, as `--app app/frontend` instead of `--app frontend`. With `web/app` and `admin/app`, `--app app` was ambiguous and ran one of them.
- Reject a query tag with a character other than ASCII letters, digits, `-`, `.`, `_` and `~`, as `query:"q&a"`. For such a tag the handler read an empty value, or the action expression failed to compile in the browser. Rename the tag, as `query:"q_a"`.

#### Generated code

Run `datapages gen` to apply these.

- Reload a page when the session of its SSE stream closes or expires. A page with only user-addressed events also reloads when its stream reconnects after the session ended. The page previously kept showing the old session's content without updates.
- Render `PageError404` with status 404 for a page load whose handler returns `datapages.ErrNotFound`, as for a URL that no route matches. For such a page load, `PageError404.GET` receives the request's query and signals and a zero path. An app without `PageError500` previously answered with the status text.
- Answer 406 Not Acceptable to a request without `Datastar-Request: true`, such as a plain form submission, on an action declared on `App` that takes `datapages.PageCacheWriter`, as on a page. Such an action that redirects answered a plain form with an HTTP redirect and dropped the queued writes, such as `ClearAll`. Submit those forms through a Datastar action.
- Replace [datastar-go](https://github.com/starfederation/datastar-go) with the Datapages SSE writer. Common events allocate nothing and take 9% to 51% of the previous time. `datastar-go`, `github.com/CAFxX/httpcompression` and `github.com/valyala/bytebufferpool` leave the application module after `go mod tidy` unless it imports them directly. Regenerate before upgrading because older generated code does not compile with this runtime.
- Stop copying strings while rendering an entry written through `datapages.PageCacheWriter`. A page load that writes 1 entry drops from 43 to 38 allocations.

#### Runtime and modules

- Label requests with the `QUERY` method as `QUERY` in the `method` label of the HTTP metrics. They were counted as `<other>`.
- Refuse a URL passed to `datapages.WithDatastarJS` that is invalid according to RFC 3986.
- Answer 404 to an asset path spelled differently from its file name, such as `/static/App.js` for `app.js`, on every system and in dev mode. Windows and macOS served such paths, unlike Linux and an `embed.FS`, and `datapages.WithAssetsCache` kept an `ETag` per spelling until exit. Spell asset links exactly like their files.
- Require a custom `messaging.Broker` to deliver messages in the order they were published, whatever their subjects. A broker that delivers each subject on a goroutine of its own breaks this.
- Require a custom `messaging.Broker` to close a subscription's channel when the subscription can no longer receive messages, such as when its connection closes. Otherwise the page stops receiving events.
- Publish through the `inmem` broker in time linear in the open streams of a page that handles an event with a `datapages.Subject` field without a `signal` tag. Such streams subscribe with a wildcard, and a publish to them took quadratic time: 12.8ms with 10,000 streams, where it takes 86us now. A stream that opened or closed meanwhile waited for it.
- Stop copying strings written to a response. A `GET /` of `example/counter` drops from 27 to 19 allocations.
- Stop copying strings written through `offline.Middleware`. A page of 22 strings drops from 35 to 14 allocations.
- Let a file served through `offline.Middleware` use sendfile when it is an `*os.File`, as a file of an `http.Dir` or a `datapages.File` opened with `os.Open` is. Embedded files cannot.
- Serve the embedded files of `datapages.WithAssets`, and the files of a `datapages.WithAssetsFS` file system other than `http.Dir`, without allocating a copy buffer of up to 32 KB per response. In a benchmark over HTTP/1.1, a request for a 30 KB file takes 18% less time.
- Compress page, action and error bodies of at least 256 bytes by default with the first accepted coding among zstd, brotli and gzip. Applications previously needed custom middleware to compress these responses. An 8 KB page of cards shrinks to 19% in 14 us. `datapages.File` and dev-mode responses remain uncompressed. A `HEAD` response gets the headers of its `GET` response. `datapages.WithResponseCompression` configures compression. `datapages.WithNoResponseCompression` disables it for loopback-only clients or body-filtering proxies. Compressing proxies pass compressed bodies through. Send pages that combine secrets with attacker-controlled input with `Cache-Control: private, no-transform` to prevent compression side channels.
- Compress static assets of at least 256 bytes by default when their type is text, JSON, JavaScript, XML, YAML, TOML, SVG, WebAssembly, a TrueType or OpenType font, an icon or a BMP image. Prefer the first accepted coding among brotli and gzip. Compress each asset/coding copy on its first request, send it from memory and write it to a configured store in the background. Copies preserve `Content-Length`, support 1 range and get distinct weak `ETag` values. Multi-range requests and dev mode use the original asset. Configure codings, levels, minimum size and storage with `assetcompress.New` and `datapages.WithAssetsCompression`. `Precompress` creates copies eagerly. `modules/assetcompress/disk` persists them across restarts and removes copies unused for 7 days by default. `datapages.WithNoAssetsCompression` disables compression, and an `AssetsCacheConfig.CacheControl` with `no-transform` keeps assets uncompressed. Keep assets immutable until server exit. A changed file of `datapages.WithAssetsFS` keeps its old `ETag`, and its compressed responses can keep its old content, until a restart. Serve such files through a GET action returning `datapages.File`, or add `datapages.WithNoAssetsCompression` and set `AssetsCacheConfig.DisableETag`.
- Let the offline worker reuse a cached same-origin asset without a request until its `max-age` passes. It asked the server on every use, also for `immutable` assets.
- Send `Cache-Control: public, max-age=0` with static assets when `datapages.WithAssetsCache` is absent or disabled. A browser could reuse a `datapages.WithAssetsFS` file after a deploy without asking, because the file had a modification time and no `Cache-Control`. A CDN now revalidates such assets too, unless `WithAssetsCache` sets a `MaxAge`.
- Send an `ETag` with static assets when `datapages.WithAssetsCache` is absent or disabled. Browsers downloaded embedded assets in full on every page load. Set `AssetsCacheConfig.DisableETag` for `datapages.WithAssetsFS` files that change.

#### `datapages init`

- Teach generated AI instructions to use isolated Web Components for browser libraries or client logic too complex for Datastar expressions. The earlier instructions did not say where such JavaScript belongs. Run `datapages init` in an existing project to update them.
- Generate a new project's templates with templ v0.3.1070 and install that version in its `.github/workflows/ci.yml`. A project scaffolded earlier keeps v0.3.1020 in its workflow: change the version in the `go install github.com/a-h/templ/cmd/templ@` step to match the templ version in its `go.mod`.

### Removed

#### `datapages lint` and `datapages gen`

- Disallow `GET` and actions on stream embeds, formerly abstract page types. They may still share `OnXXX`, `StreamOpen` and `StreamClose`. Move `GET` to each page, using a shared helper for common code. Move actions to one page or to `*App` when several pages use them.

### Fixed

#### `datapages lint` and `datapages gen`

- Reject a page that reaches the same stream embed through 2 embedded types at the same depth. The generated code for such a page did not compile: Go reported an ambiguous selector. Declare the handler on the page, which shadows the embedded ones.
- Reject a page or stream embed that embeds a type of another package, such as `sync.Mutex`. Declare handlers in the app package and move other fields to `App`.
- Reject a file of the app package with a `//go:build` line or a name such as `probe_windows.go`, except a test file and a file under `//go:build ignore`. `datapages gen` left out its pages, actions and events without an error on a platform that excludes the file. Move the file to another package, or remove its constraint.
- Report a missing app package directory, such as `./app` in a module without a `datapages.NewServer` call. The package of the parent directory was read as the app instead, and `datapages gen` wrote stubs into the missing directory.
- Refuse a `watch.app-host` without an `http` or `https` scheme and a host, such as `localhost:8080`, in `datapages watch`. It was read as a URL without a host, which the dev server could not send requests to. Write it as `http://localhost:8080`.
- Apply `watch.log.level` in `datapages watch`. `erronly`, the default, logged info like `verbose`. It now logs errors only.
- Reject an `OnXXX` handler that takes a second `datapages.SSE`, session or `datapages.StreamID` parameter, as every other handler does. A field with 2 names, such as `sse, sse2 datapages.SSE`, declares 2 parameters: they were read as one, and the generated call did not compile.
- Reject an `OnXXX` handler that takes its event by pointer, as in `event *EventPing`. The generated code for it did not compile. Take the event by value.
- Reject a route containing `"` or `\`. For such a route, `datapages gen` failed with a Go syntax error, or the generated server matched a different path when the backslash formed a Go escape such as `\u00e9`. Write the characters as `%22` and `%5C`, which `net/http` decodes before matching the route.
- Reject a route containing a backtick. On a page with a stream or a query field that reflects a signal, `datapages gen` failed with a Go syntax error. Write it as `%60`, which `net/http` decodes before matching the route.
- Reject an unexported defined type as the type argument of `datapages.Path`, `datapages.Query` or `datapages.Signals`, including one behind an exported alias. The generated code for such a handler did not compile. Export the type.
- Reject a route containing `?`, `#` or a `%` without 2 hex digits after it. The generated links, action expressions and stream URL carried the route as written, where the browser read `?` as the start of the query and `#` as the fragment, and `net/http` answered 400 to the `%`: none of them reached the page. Write the characters as `%3F`, `%23` and `%25`, which `net/http` decodes before matching the route.
- Reject an event with more than 1 `datapages.SubjectUser` field, such as `To, Cc datapages.SubjectUser`. Only the first field routed the event: a user named in a later field never received it, and the user in the first field received it whatever the others held. Keep 1 `SubjectUser` field and dispatch the event once per user.
- Reject an error-prone event field that holds a subject type, such as `Recipients []datapages.SubjectUser`. It looks like a list of recipients, but it was a payload field, and the event went to every stream of the pages handling it. Declare 1 `datapages.SubjectUser` field and dispatch once per user.
- Reject a page other than `PageIndex` at `/`, as in `// PageHome is /`. The router sent every request for `/` to that page, and `PageIndex` never rendered. Give the page another route, or move its handlers to `PageIndex`.
- Report an action of a page whose route ends in a `{name...}` wildcard, as in `// PageFiles is /files/{rest...}`, with an error saying that such a page cannot have actions. The report was a route conflict about a pattern the app does not declare, or a missing path comment with a suggested route that failed the same way. Remove the wildcard from the page route to add actions.
- Report the same conflicts on every run when handlers take `datapages.Session` with different data types. The data type of whichever page was read first won, which changed from run to run which handlers were reported. The first page by name now declares the type.
- Report the errors of a struct type that several events share, such as a field without a `json` tag, in the same order on every run: by event name.
- Accept a handler parameter typed with an alias of `datapages.Path`, `datapages.Query`, `datapages.Signals` or `datapages.State`, as in `type FormSignals = datapages.Signals[struct{...}]`. The check read the `Values` field of the wrapper instead of its type argument and reported errors located in `datapages.go`, and refused an alias of `datapages.State` as not a struct.
- Report an action route that ends in a `{name...}` wildcard, as in `// POSTUpload is /upload/{path...}`, with an error saying that an action route cannot end in a wildcard. The error was a route conflict on `POST /upload/{path...}/{$}`, a pattern the app does not declare. Pass the value in a query parameter or a signal instead.
- Reject an assets URL prefix that contains `%`, as in `// StaticFS is /my%20files/`. The server answered 404 to every request under such a prefix. Write the prefix without percent-encoding, such as `/my-files/`.
- Accept in an assets URL prefix the characters its error message lists, ASCII letters, digits, `-`, `.`, `_`, `~` and `/`, and no others. Quotes and other punctuation the message did not list were accepted, and a segment starting with a dot, such as `/.well-known/`, was refused.
- Reject a `//go:embed` pattern on the assets variable that is a glob or names a single file, as in `//go:embed static/*.css` or `//go:embed static/app.css`. The server used the pattern as the directory to serve and answered 404 for every asset. Embed the directory, as in `//go:embed static` or `//go:embed static/*`.
- Stop reporting a valid `href={ LoginURL }` as relative on some runs when a function also declares a local `LoginURL` constant.
- Report each mismatch between the route and the `datapages.Path` struct of an action on `App` as its own error, at the path parameter or the field,, as for a page. They were joined into 1 error at the method name.
- Accept a `datapages.NewServer` call with its fifth type argument written out, as in `datapages.NewServer[app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server, *gen.Server]`. It was refused with "datapages.NewServer needs four type arguments, got 5".
- Accept an app package whose name differs from its directory, as `package app` in `go-app/`, when the file calling `datapages.NewServer` imports it without an alias. The app type was refused with "must live in its own package".
- Read the `datapages.NewServer` call in a file whose `//go:build` line negates a tag, as `//go:build !windows`. `datapages gen` skipped the file and generated code as if the module had no call.
- Report a problem in a `datapages.NewServer` call with a path relative to the module root, as every other error is. A wrong number of type arguments, a wrong `Metrics` type argument and a file that does not parse were reported with an absolute path.
- Check the attributes inside a conditional attribute of a `.templ` file, as in `<a if ok { href="/login" }>`, in both branches. A hardcoded `href`, a form `action`, a hardcoded Datastar action URL or an action of another page went unreported there.
- Include the HTTP method in the action names that route errors show, as in `PageUser.DELETEFollow`. Before, they showed `PageUser.Follow`, which could also mean `PageUser.POSTFollow`.
- Show the line number of a Go build error that comes without a column. Before, it showed line 0, and on Windows the drive letter as the file name.
- Check the `.templ` files of the packages that the app package imports from its module, such as `app/template`. Before, only the app package's own templates were checked. A wrong `href`, a form `action`, a hardcoded action URL or another page's action in those templates went unreported.
- Reject an action that takes `datapages.SSE` and returns a body. Its HTML went into the open event stream as broken events, and with compression the browser could not read the stream at all. Send the body with `sse.PatchElement` instead.
- Reject a page whose events need the same signal both as a value and as an object, such as an event field tagged `signal:"chat"` and another tagged `signal:"chat.room"`. `$chat.room` exists only when `$chat` is an object, and the stream of such a page never opened. Bind one of the fields to another signal.
- Name the event whose subject must change in the fix for overlapping event subjects. When the event with subject fields sorted second, the fix named the other event.
- Suggest the valid subject types, including `datapages.SubjectStateID`, for a payload field named like a subject field, such as `SubjectUser []string`. The fix named the nonexistent `datapages.Subjects` and `datapages.SubjectUsers` types.
- Spell `datapages.SubjectUser` correctly in the fix for a `signal` tag on that field. It was written as `datapages.SubjectUser(s)`.
- State the accepted syntax in the fix for an invalid `signal` tag: each period-separated segment must match `[A-Za-z_][A-Za-z0-9_]*` and contain no `__`. The fix incorrectly required a lowercase first letter.
- Base the suggested route for an action with an invalid route comment on its page route. For a page at `/user/{name}`, the fix suggested `/user/follow` instead of `/user/{name}/follow`.
- Base the suggested route for an action without a route comment on the route suggested for its page when the page also has no route comment. For `PageUser.POSTFollow`, the fix suggested `/follow` instead of `/user/follow`.
- Report a subject that 2 applications of a module claim in `datapages lint` when 1 of them has other errors, mirroring `datapages gen` behavior.
- Reject a `reflectsignal` query field that can't be written as text, also when the `GET` takes no `Signals`. Its value grew in the URL on every reload.
- Accept an event or signals field of a struct type from another module, such as `url.URL` or `sql.NullString`. Its fields were held to the `json` tag rules, with errors pointing into the standard library.
- Keep the generated code when `datapages gen` rejects the `SessionData` type argument of `datapages.NewServer`. It was replaced and did not compile with the call.
- Keep the generated code of 2 applications whose events claim the same subject. It was replaced before the error was reported.
- Replace generated files only once all are written. A full disk or a killed `datapages gen` could leave a package that did not compile.
- Rerun `datapages gen` in `datapages watch` when the package calling `datapages.NewServer` changes. A new `Metrics` type argument took effect only after the next change to the app package.
- Accept a `reflectsignal` query field whose signal is a pointer, such as an `int` query field reflected into a `*int` signal. It was refused with "incompatible JSON kinds", although `encoding/json` decodes the value into it.
- Accept a string query field reflected into a signal that implements `encoding.TextUnmarshaler`, such as an `int64` type with an `UnmarshalText` method. It was refused with "incompatible JSON kinds", although `encoding/json` passes the string to `UnmarshalText`.
- Reject a number or bool query field reflected into a signal that implements `encoding.TextUnmarshaler` and not `json.Unmarshaler`. `encoding/json` decodes only strings into such a type, and an action that took the signal answered 400. Reflect a string field or a field that implements `encoding.TextMarshaler`.
- Show the update notice in `datapages watch` when the running version is a prerelease of the latest release, such as `0.11.0-rc.1` of `v0.11.0`. The check ignored the prerelease suffix and found both versions equal.

#### Generated code

Run `datapages gen` to apply these.

- Reconnect a page's SSE stream whenever it ends, without a retry limit. Some endings, such as a graceful shutdown, previously left the page without live updates.
- Write the CSRF script into every request-specific page and action response whose actions carry a session cookie, including documents whose handlers do not accept a session. This prevents actions submitted from those documents from returning 403. `PageOffline` and page-cache entries omit the script because visitors share them.
- Generate code that compiles when `App.Head` takes the session before the `*http.Request` in an app that uses `datapages.PageCacheWriter`.
- Generate code that compiles when the type argument of `datapages.Path`, `datapages.Query` or `datapages.Signals` is an alias of a defined type, as in `type Filter = SearchQuery`.
- Generate code that compiles when the type argument of `datapages.Path`, `datapages.Query` or `datapages.Signals` is a struct type literal with an embedded field, such as `time.Time`.
- Generate code that compiles when the session data type or a field type of `datapages.Path`, `datapages.Query` or `datapages.Signals` is an alias declared in a package with the same name as one the generated code imports, such as `stream`.
- Generate code that compiles when `PageError404.GET` takes a path, query, signals or dispatcher parameter. For a URL that no route matches, the handler receives that URL's query and signals and a zero path.
- Generate code that compiles when a path, query or signals field type comes from a package named `path`, `query`, `signals` or after another variable the generated handlers declare.
- Answer a page load whose handler returns `datapages.ErrBadRequest`, `datapages.ErrForbidden`, `datapages.ErrNotFound` or `datapages.ErrConflict` with that status in an app that defines `PageError500`, as every app created by `datapages init` does. Such a page load got `PageError500` with status 500, and no page could answer 404. `PageError500` now renders for status 500 only. Other statuses get their status text, except 404 in an app that defines `PageError404`.
- Keep the SSE streams of a page with both public and user-addressed events open while the tab is hidden when its `GET` returns `enableBackgroundStreaming` set to `true`, for signed-in visitors and guests alike. Such a page dropped its reload on visibility but still closed its stream, which left it without the events published while the tab was hidden.
- Escape a `'` in a route, as in `// PageAuthor is /o'reilly`, where the generated code writes the route into JavaScript strings: the page's stream URL, its action expressions and the URL a `reflectsignal` query field rewrites. The quote ended the string, which left the stream and the actions of such a page failing with a syntax error in the browser.
- Recover a panic in an `OnXXX` handler on the stream that a page with both public and user-addressed events serves to signed-out visitors, as the stream of signed-in visitors does. The panic dropped the connection, skipped `StreamClose` and never reached `RecoverError`, which leaked per-tab state that `StreamOpen` registered for every such guest.
- Send `Cache-Control: private` with a page rendered for a signed-in visitor, the 404 and error pages included, unless the response already has a `Cache-Control` header. Such a page carries a CSRF token derived from the session. A shared cache that stored the page served it to other visitors, and their actions failed with 403.
- Set the session cookie of an action that takes `datapages.PageCacheWriter` and returns `newSession` or `closeSession` but neither a redirect nor a body. The response went out without the cookie: a sign-in left the visitor signed out, and a sign-out left them signed in.
- Answer with the error status when an action that takes `datapages.PageCacheWriter` and returns neither a redirect nor a body fails or panics. The response was 200, with the error status written as text into the event stream.
- Read a subject field bound to a nested signal, as `signal:"chat.room"`, from the object Datastar sends, `{"chat":{"room":"lobby"}}`. The stream of a page handling such an event answered 400 on every connect.
- Preserve `json.RawMessage` content after a standalone carriage return in `SSE.PatchSignals` and `SSE.PatchSignalsIfMissing`.
- Reduce memory held by compressed SSE streams. A zstd stream holds about 1.5 MiB instead of brotli's 2 to 11 MiB, and encoders are pooled instead of allocating 6.3 MiB before each response. The default order is zstd, brotli and gzip. `datapages.WithSSECompression` configures the order, levels and windows. Compressed responses include `Vary: Accept-Encoding` and end their compressed data. For a 99 KB list re-render with 1 changed item, zstd sends 325 B instead of 22 B. Prefer brotli or enlarge `ZstdWindow` to send fewer bytes.
- Skip `RecoverError` after an HTML response body starts. Its SSE events previously corrupted a partial HTML response.
- Recover and log a panic in `RecoverError` once. It previously invoked `RecoverError` again and could write 2 compressed streams in 1 response.
- Send `Vary: X-Datapages-Offline-Version` before opening an SSE response from an action that takes `datapages.PageCacheWriter`.
- Log errors from bodies passed to `PageCacheWriter.Set` or `SetShim` when rendering an action's event stream or redirect script.
- Log the stack of a panic in `StreamOpen`, as for a panic in `GET`, an action or `OnXXX`. The log previously held only the panic value.
- Stop sending a page cache entry again on every load when its version is above 2^53, such as a 64-bit hash.
- Use the app package's own name in the error `datapages.NewServer` returns for an unexpected `WithSessionManager`, and in the example of `Init`. Both said `dpapp`, an alias internal to the generated code.
- Stop reloading a page with only user-addressed events for signed-out visitors on every tab switch. Each reload discarded client-side state.
- Give signed-out visitors of a stateful page with only user-addressed events a stream at `_$/anon/`. Their state actions answered 409. `StreamOpen` and `StreamClose` run on it with a guest session.

#### Runtime and modules

- Leave static assets unedited in `offline.Middleware`. An HTML asset got the connectivity and registration scripts, appended at its end when it had no `</head>` or `</body>`, as a fragment loaded with `fetch` has. The scripts also changed the bytes behind the asset's `ETag`.
- Percent-encode the characters a URL path cannot carry, such as `#`, `?` and `%`, in the file names that `assets.Path` and `href.Asset` turn into URLs. `assets.Path("a#b.css")` returned `/static/a#b.css`, which a browser requests as `/static/a`, and a `%` in a file name made the server answer 400.
- Label the HTTP metrics with the route when a middleware added with `datapages.WithMiddleware` passes on a new request, as `r.WithContext` returns when storing a CSP nonce. The request counter and the latency histogram labelled every request through such a middleware `<unmatched>`.
- Accept `&` and `'` in the URL passed to `datapages.WithDatastarJS`, as in `https://cdn.example.com/datastar.js?v=1&min=1`. RFC 3986 allows both, and the page writes the URL escaped. Such a URL was refused.
- Deliver events through the `natscore` broker in the order they were dispatched, as `inmem` does. 2 events of different types dispatched one after the other often arrived in reverse order.
- End the SSE streams subscribed through the `natscore` broker when its NATS connection closes, which nats.go does after the last failed reconnect attempt: 60 attempts 2s apart with the defaults of `nats.Connect`. Open pages kept their streams and received no more events, without an error. They now reconnect, and the server logs each reconnect that fails on the closed connection.
- Return from `natskv.SessionManager.UserSessions` and `CloseAllUserSessions` the token the session's cookie carries, as `inmem` does. In v0.10.0 and v0.10.1, each call returned a new token for the same session, and an application that compares a listed token with `Session.Token()` to find the current session never found it. A session created before the upgrade, or under one of the `PreviousEncryptionKeys`, keeps a cookie that differs from its listed token until the user signs in again.
- Return from `natskv.SessionManager.UserSessions` an iterator that yields the sessions on every range, as `inmem` does. A second range yielded nothing, and an iterator never ranged kept a KV watcher subscribed until the context passed to `UserSessions` ended.
- Return an error from `natskv.SessionManager.UserSessions` and `DeleteExpired` when their context ends, NATS stalls for 5s or the connection closes during the call. `UserSessions` returned the sessions read by then as the complete list, and `DeleteExpired` returned nil with expired sessions left in the bucket.
- Report a token that can't be decrypted as `sessions.ErrSessionNotFound` in `natskv.SessionManager.Session`, as `inmem` does. This includes forged tokens and cookies encrypted with a key that is no longer in `PreviousEncryptionKeys`. Before, `errors.Is(err, sessions.ErrSessionNotFound)` was false for them.
- Send what net/http logs, such as a panic in a handler or a failed TLS handshake, to the logger set with `datapages.WithLogger`, at error level. It went to stderr as JSON at info level, and the metrics server of `datapages.WithPrometheus` wrote it as plain text. A server passed to `datapages.WithHTTPServer` keeps its own `ErrorLog` if it sets one.
- Register the offline service worker for the whole origin when `offline.Config.ScriptURL` is in a subdirectory, such as `/static/sw.js`. The browser limited such a worker to the pages under that directory. Other pages were not available offline, and their `datapages.PageCacheWriter` writes never reached the worker.
- Refuse an `offline.Config.ScriptURL` that is not a plain path, such as `sw.js` or `/sw.js?v=2`. The worker never loaded from those. `offline.WithServiceWorker` and `WithOffline` return an error, and `offline.Middleware` panics. Use a path that starts with `/` and holds only `/` and the unreserved characters of RFC 3986.
- Stop logging a panic, `response writer failed to flush`, with a stack trace when a visitor leaves a page while its SSE stream opens. A stream on the `natscore` broker waits for a round trip to NATS before it opens, which makes this more likely.
- Keep a response marked `Cache-Control: no-store` out of the offline worker's cache, and fetch a stored response marked `no-cache` from the network first, using the stored copy only offline. The worker stored every same-origin response that an element or `fetch` loaded, such as an image, whatever its `Cache-Control`, and served the stored copy first. Increase `offline.Config.WorkerVersion`, which deletes what the previous worker stored.
- Remove a response from the offline worker's cache when the network answers 401, 403, 404 or 410. The worker kept serving the stored copy of a file after the server stopped authorizing it or deleted it, until `offline.Config.WorkerVersion` changed. Increase `offline.Config.WorkerVersion`, which deletes what the previous worker stored.
- Stop the offline worker from copying a range response with status 206, such as a video that a `video` element streams, for its cache, which refuses it. Nothing read the copy, which can keep every byte the page has read in memory.
- End the response of a static asset when its client accepts no more of the file for 1 minute. A client that stopped reading held the request and the open file for as long as it kept the connection. `datapages.WithFileSendTimeout` changes the time, which also applies to `datapages.File`.
- Abort SSE writes blocked on clients during graceful shutdown. They previously held shutdown until its timeout, skipped `StreamClose`, and could race with net/http cleanup.
- Stop `offline.Middleware` from logging `http: superfluous response.WriteHeader call` for each SSE response.
- Stop `offline.Middleware` from adding its scripts to a range response, status 206, of an HTML file, such as a static asset that a download manager resumes. The body was longer than its `Content-Range` said, and the saved file was corrupt.
- Stop rounding an `offline.Config.WorkerVersion` above 2^53 in the browser. A version rounded down added the registration script to every page.
- Treat `/tickets` and `/tickets/` as the same page in the offline cache, as the server does. A visit to `/tickets` missed the copy cached for `/tickets/`, so the page didn't load offline, and the server sent the copy again on every online visit.
- Count in `suppressed` every record that `datapages.WithLogSampling` holds back. A record logged at the same moment as the first one of its kind was dropped uncounted.
- Stop appending the 500 page to a page that fails halfway through rendering behind `WithOffline` or `offline.Middleware`. The visitor got the half page and a second document after it.
- Refuse in `datapages.NewServer` a `ServerConfig` value that a custom `ServerOption` sets and the field's `With` option refuses. A session cookie name with a space, for example, passed and kept every visitor signed out.
- Apply the defaults of `datapages.WithPrometheus` to a `datapages.PrometheusConfig` that a custom `ServerOption` writes into `ServerConfig.Prometheus`. Without a `Registerer`, `NewServer` panicked, without a `Gatherer`, every scrape of `/metrics` panicked, and no HTTP request was counted.
- Serve font assets with their `font/*` type on every host. Some mime tables, such as shared-mime-info on Linux, typed `.otf` and `.otc` fonts as OpenDocument templates, which also kept them uncompressed.

#### `datapages init`

- Pass the app's `embed.FS` assets variable to `datapages.WithAssets` when creating a missing `cmd/server/main.go`. Generated asset URLs previously returned 404. An existing `main.go` is not rewritten; add `datapages.WithAssets(app.StaticFS, false)` manually.
- Shut down gracefully on SIGTERM in the `cmd/server/main.go` it writes. Docker, Kubernetes and systemd stop a process with SIGTERM, which previously ended the server without waiting for requests, SSE streams and `StreamClose` hooks. It does not rewrite an existing `main.go`: add `syscall.SIGTERM` to its `signal.NotifyContext` call.
- Check out the repository before installing Go in `.github/workflows/ci.yml`, which lets `actions/setup-go` restore its cache. In a project scaffolded earlier, move the `Checkout repository` step above `Install Go`.
- Use the enclosing Go module when run in a subdirectory, as `datapages gen` does, instead of creating a nested module there.

### Security

#### Generated code

Run `datapages gen` to apply these.

- Prevent carriage returns in `SSE.PatchElement`, `SSE.PatchElementAt` and `SSE.ExecuteScript` content from injecting SSE fields and JavaScript. An attacker who controls text rendered in a patched component could run JavaScript in each recipient's session. Applications using these methods are affected in v0.10.0 and v0.10.1; applications calling datastar-go's equivalent methods are affected in v0.9.4 and earlier. Upgrade, regenerate and redeploy. Until then, remove `\r` from untrusted text before rendering it.

#### Runtime and modules

- Prevent `natskv.SessionManager.CloseAllUserSessions` from returning nil while sessions of the user stay signed in. When its context ended during the call, as a request context does when the client disconnects, it closed only the sessions read by then. Whoever held a cookie of the others, such as an attacker the user meant to sign out, stayed signed in. A NATS stall of 5s during the call and a closed connection did the same. The end of the context no longer stops the call, and the other 2 return an error. Applications calling `CloseAllUserSessions` are affected in v0.1.0 through v0.10.1. Upgrade and redeploy. Until then, pass it `context.WithoutCancel(r.Context())`, which covers the client that disconnects.
- Stop sending a signed-in user's CSRF token to other origins. A `@post`, `@put`, `@patch` or `@delete` action to another origin sent it the token if its CORS policy allowed the `X-CSRF-Token` header. This affects v0.1.0 through v0.10.1. Upgrade and redeploy. Until then, call other origins without a Datastar action.

## [0.10.1] - 2026-09-26

### Added

- Count streams that end at their session's `ExpiresAt` under `reason="expired"` in `datapages_sse_disconnects_total`.

### Changed

- Preserve an existing `AGENTS.md` without a datapages stamp when running `datapages init`. To replace the unstamped `AGENTS.md` generated by v0.10.0, delete it before running `datapages init`.

### Fixed

- Prevent `offline.Middleware` from corrupting or dropping a response when text before `</head>` contains invalid UTF-8 or a character whose lowercase form has a different byte length, such as `İ`. This affects `offline.WithServiceWorker`, `offline.Middleware` and the generated `WithOffline` in v0.10.0.
- Stop `datapages init` from backing up unedited generated skills and `AGENTS.md`. Generated instruction files include a content hash so later runs can detect user edits. Unedited skills from v0.10.0 are replaced without a backup too.

### Security

- Prevent signed-in users from receiving private events addressed to another user when their streams share the event's signal value. This affects v0.10.0. It also affects v0.7.0 through v0.9.4 when the page handles a public signal-scoped event. Upgrade to v0.10.1 and run `datapages gen`.
- Prevent attacker-controlled values in an `SSE.Prefetch` URL from running JavaScript (XSS) in browsers that receive the prefetch. Applications are affected when they build the URL from untrusted data without percent-encoding it. This affects v0.10.0. Upgrade to v0.10.1 and redeploy. Until then, use the `href` builders or escape each value with `url.PathEscape` or `url.QueryEscape`.
- Prevent `natskv.SessionManager.SaveSession` from re-creating a session deleted by `CloseSession`, `CloseAllUserSessions` or `DeleteExpired`. A stale save could let a signed-out user or an attacker with a revoked cookie authenticate again. Applications using `SaveSession` are affected in v0.1.0 through v0.10.0. Upgrade to v0.10.1 and redeploy. There is no workaround.
- End a user's private-event streams at the session's `ExpiresAt`. In v0.10.0, a client that opened a stream before expiry, including one using a stolen cookie, could keep reading afterward until the session was deleted. Applications setting a non-zero `NewSession.ExpiresAt` are affected. Upgrade to v0.10.1, run `datapages gen` and redeploy. Until then, call `DeleteExpired` frequently; the inmem and natskv stores end streams when they delete sessions.

[Unreleased]: https://github.com/romshark/datapages/compare/v0.10.1...HEAD
[0.10.1]: https://github.com/romshark/datapages/compare/v0.10.0...v0.10.1
