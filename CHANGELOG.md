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
- Accept `GETXXX` actions, which answer `GET` with a file such as an image or a download. Declare one on a page or on `App`, as in `// GETImage is /images/{name}`, and return a `datapages.File`. A GET action takes `*http.Request`, the session, `datapages.Path` and `datapages.Query`. It runs no cross-origin or CSRF check and must not change server state. Build its URL with the `href` package: `href.App.Image(name)` for `(*App).GETImage`, `href.PageDoc.Export(id)` for `PageDoc.GETExport`.
- Accept `datapages.File` as the return value of any action, to answer with bytes instead of a document. Return it alone or with `error`. `net/http.ServeContent` serves it, which answers `HEAD`, `Range` and `If-Modified-Since`, and the response carries `X-Content-Type-Options: nosniff`. An error answers with its status and the status text. Only a request with `Sec-Fetch-Dest: document`, such as a link opened in a tab, gets `PageError404` or `PageError500`.

#### Runtime and modules

- Reload open pages when a deployment changes the build. Set `datapages.WithBuildID` when replicas run different binaries of one release. A `Content-Security-Policy` must allow the inline build script.

### Changed

#### `datapages lint` and `datapages gen`

- Reject actions, `OnXXX` handlers, `StreamOpen`, `StreamClose` and `State[T]` on a page whose route ends in `{$}`, as `// PageUser is /user/{name}/{$}`. A `{$}` at the end of a page route declares a page that serves only `GET`. An action below such a route got an error saying that it was not under its page, with a suggested route that net/http rejects. Remove `{$}` from the route of such a page: it keeps its URL.
- Read an exported method on `App` named `QUERY` followed by an uppercase letter, such as `QUERYStats`, as an action. Such a method is rejected when it's a helper rather than an action: rename it.
- Read an exported method on `App` named `GET` followed by an uppercase letter, such as `GETStats`, as a GET action. Such a method is rejected when it's a helper rather than an action: rename it.

#### Generated code

Run `datapages gen` to apply these.

- Reload a page when the session of its SSE stream closes or expires. A page with only user-addressed events also reloads when its stream reconnects after the session ended. The page previously kept showing the old session's content without updates.
- Render `PageError404` with status 404 for a page load whose handler returns `datapages.ErrNotFound`, as for a URL that no route matches. For such a page load, `PageError404.GET` receives the request's query and signals and a zero path. An app without `PageError500` previously answered with the status text.
- Answer 406 Not Acceptable to a request without `Datastar-Request: true`, such as a plain form submission, on an action declared on `App` that takes `datapages.PageCacheWriter`, as on a page. Such an action that redirects answered a plain form with an HTTP redirect and dropped the queued writes, such as `ClearAll`. Submit those forms through a Datastar action.

#### Runtime and modules

- Label requests with the `QUERY` method as `QUERY` in the `method` label of the HTTP metrics. They were counted as `<other>`.
- Refuse a URL passed to `datapages.WithDatastarJS` that is invalid according to RFC 3986.
- Require a custom `messaging.Broker` to deliver messages in the order they were published, whatever their subjects. A broker that delivers each subject on a goroutine of its own breaks this.
- Require a custom `messaging.Broker` to close a subscription's channel when the subscription can no longer receive messages, such as when its connection closes. Otherwise the page stops receiving events.
- Publish through the `inmem` broker in time linear in the open streams of a page that handles an event with a `datapages.Subject` field without a `signal` tag. Such streams subscribe with a wildcard, and a publish to them took quadratic time: 12.8ms with 10,000 streams, where it takes 86us now. A stream that opened or closed meanwhile waited for it.
- Stop copying strings written to a response. A `GET /` of `example/counter` drops from 27 to 19 allocations.
- Stop copying strings written through `offline.Middleware`, and let files served through it use sendfile. A page of 22 strings drops from 35 to 14 allocations.
- Serve the embedded files of `datapages.WithAssets`, and the files of a `datapages.WithAssetsFS` file system other than `http.Dir`, without allocating a copy buffer of up to 32 KB per response. In a benchmark over HTTP/1.1, a request for a 30 KB file takes 18% less time.

#### `datapages init`

- Generate a new project's templates with templ v0.3.1070 and install that version in its `.github/workflows/ci.yml`. A project scaffolded earlier keeps v0.3.1020 in its workflow: change the version in the `go install github.com/a-h/templ/cmd/templ@` step to match the templ version in its `go.mod`.

### Fixed

#### `datapages lint` and `datapages gen`

- Reject a page that embeds two types declaring one action at the same depth, or that reaches one abstract page through two embedded types at the same depth. The generated code for such a page did not compile: Go reported an ambiguous selector. Declare the handler on the page, which shadows the embedded ones.
- Reject an `OnXXX` handler that takes a second `datapages.SSE`, session or `datapages.StreamID` parameter, as every other handler does. A field with two names, such as `sse, sse2 datapages.SSE`, declares two parameters: they were read as one, and the generated call did not compile.
- Reject an `OnXXX` handler that takes its event by pointer, as in `event *EventPing`. The generated code for it did not compile. Take the event by value.
- Reject a route containing `"` or `\`. For such a route, `datapages gen` failed with a Go syntax error, or the generated server matched a different path when the backslash formed a Go escape such as `\u00e9`. Write the characters as `%22` and `%5C`, which `net/http` decodes before matching the route.
- Reject a route containing a backtick. On a page with a stream or a query field that reflects a signal, `datapages gen` failed with a Go syntax error. Write it as `%60`, which `net/http` decodes before matching the route.
- Reject an unexported defined type as the type argument of `datapages.Path`, `datapages.Query` or `datapages.Signals`, including one behind an exported alias. The generated code for such a handler did not compile. Export the type.
- Reject a route containing `?`, `#` or a `%` without two hex digits after it. The generated links, action expressions and stream URL carried the route as written, where the browser read `?` as the start of the query and `#` as the fragment, and `net/http` answered 400 to the `%`: none of them reached the page. Write the characters as `%3F`, `%23` and `%25`, which `net/http` decodes before matching the route.
- Reject an event with more than one `datapages.SubjectUser` field, such as `To, Cc datapages.SubjectUser`. Only the first field routed the event: a user named in a later field never received it, and the user in the first field received it whatever the others held. Keep one `SubjectUser` field and dispatch the event once per user.
- Reject an error-prone event field that holds a subject type, such as `Recipients []datapages.SubjectUser`. It looks like a list of recipients, but it was a payload field, and the event went to every stream of the pages handling it. Declare one `datapages.SubjectUser` field and dispatch once per user.
- Reject a page other than `PageIndex` at `/`, as in `// PageHome is /`. The router sent every request for `/` to that page, and `PageIndex` never rendered. Give the page another route, or move its handlers to `PageIndex`.
- Report an action of a page whose route ends in a `{name...}` wildcard, as in `// PageFiles is /files/{rest...}`, with an error saying that such a page cannot have actions. The report was a route conflict about a pattern the app does not declare, or a missing path comment with a suggested route that failed the same way. Remove the wildcard from the page route to add actions.
- Report the same conflicts on every run when handlers take `datapages.Session` with different data types. The data type of whichever page was read first won, which changed from run to run which handlers were reported. The first page by name now declares the type.
- Report the errors of a struct type that several events share, such as a field without a `json` tag, in the same order on every run: by event name.
- Accept a handler parameter typed with an alias of `datapages.Path`, `datapages.Query`, `datapages.Signals` or `datapages.State`, as in `type FormSignals = datapages.Signals[struct{...}]`. The check read the `Values` field of the wrapper instead of its type argument and reported errors located in `datapages.go`, and refused an alias of `datapages.State` as not a struct.
- Report an action route that ends in a `{name...}` wildcard, as in `// POSTUpload is /upload/{path...}`, with an error saying that an action route cannot end in a wildcard. The error was a route conflict on `POST /upload/{path...}/{$}`, a pattern the app does not declare. Pass the value in a query parameter or a signal instead.
- Stop reporting "GET handler must return body datapages.Component" next to the actual error when a `GET` declared on an embedded type has an invalid parameter.
- Reject an assets URL prefix that contains `%`, as in `// StaticFS is /my%20files/`. The server answered 404 to every request under such a prefix. Write the prefix without percent-encoding, such as `/my-files/`.
- Accept in an assets URL prefix the characters its error message lists, ASCII letters, digits, `-`, `.`, `_`, `~` and `/`, and no others. Quotes and other punctuation the message did not list were accepted, and a segment starting with a dot, such as `/.well-known/`, was refused.
- Reject a `//go:embed` pattern on the assets variable that is a glob or names a single file, as in `//go:embed static/*.css` or `//go:embed static/app.css`. The server used the pattern as the directory to serve and answered 404 for every asset. Embed the directory, as in `//go:embed static` or `//go:embed static/*`.
- Stop reporting a valid `href={ LoginURL }` as relative on some runs when a function also declares a local `LoginURL` constant.
- Report each mismatch between the route and the `datapages.Path` struct of an action on `App` as its own error, at the path parameter or the field,, as for a page. They were joined into one error at the method name.
- Accept a `datapages.NewServer` call with its fifth type argument written out, as in `datapages.NewServer[app.App, datapages.DisableSessions, datapages.DisablePrometheus, gen.Server, *gen.Server]`. It was refused with "datapages.NewServer needs four type arguments, got 5".
- Accept an app package whose name differs from its directory, as `package app` in `go-app/`, when the file calling `datapages.NewServer` imports it without an alias. The app type was refused with "must live in its own package".
- Report a problem in a `datapages.NewServer` call with a path relative to the module root, as every other error is. A wrong number of type arguments, a wrong `Metrics` type argument and a file that does not parse were reported with an absolute path.
- Check the attributes inside a conditional attribute of a `.templ` file, as in `<a if ok { href="/login" }>`, in both branches. A hardcoded `href`, a form `action`, a hardcoded Datastar action URL or an action of another page went unreported there.
- Include the HTTP method in the action names that route errors show, as in `PageUser.DELETEFollow`. Before, they showed `PageUser.Follow`, which could also mean `PageUser.POSTFollow`.
- Show the line number of a Go build error that comes without a column. Before, it showed line 0, and on Windows the drive letter as the file name.
- Check the `.templ` files of the packages that the app package imports from its module, such as `app/template`. Before, only the app package's own templates were checked. A wrong `href`, a form `action`, a hardcoded action URL or another page's action in those templates went unreported.
- Reject an action that takes `datapages.SSE` and returns a body. Its HTML went into the open event stream as broken events, and with compression the browser could not read the stream at all. Send the body with `sse.PatchElement` instead.
- Reject a page whose events need one signal both as a value and as an object, such as an event field tagged `signal:"chat"` and another tagged `signal:"chat.room"`. `$chat.room` exists only when `$chat` is an object, and the stream of such a page never opened. Bind one of the fields to another signal.

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
- Keep the action of an embedded type when the page, or the embedded type, declares an action with the same suffix for another HTTP method, such as an embedded `PUTSave` next to `POSTSave`. The embedded action was dropped: the server did not route it and the `action` package had no builder for it.
- Keep the SSE streams of a page with both public and user-addressed events open while the tab is hidden when its `GET` returns `enableBackgroundStreaming` set to `true`, for signed-in visitors and guests alike. Such a page dropped its reload on visibility but still closed its stream, which left it without the events published while the tab was hidden.
- Escape a `'` in a route, as in `// PageAuthor is /o'reilly`, where the generated code writes the route into JavaScript strings: the page's stream URL, its action expressions and the URL a `reflectsignal` query field rewrites. The quote ended the string, which left the stream and the actions of such a page failing with a syntax error in the browser.
- Recover a panic in an `OnXXX` handler on the stream that a page with both public and user-addressed events serves to signed-out visitors, as the stream of signed-in visitors does. The panic dropped the connection, skipped `StreamClose` and never reached `RecoverError`, which leaked per-tab state that `StreamOpen` registered for every such guest.
- Send `Cache-Control: private` with a page rendered for a signed-in visitor, the 404 and error pages included, unless the response already has a `Cache-Control` header. Such a page carries a CSRF token derived from the session. A shared cache that stored the page served it to other visitors, and their actions failed with 403.
- Set the session cookie of an action that takes `datapages.PageCacheWriter` and returns `newSession` or `closeSession` but neither a redirect nor a body. The response went out without the cookie: a sign-in left the visitor signed out, and a sign-out left them signed in.
- Answer with the error status when an action that takes `datapages.PageCacheWriter` and returns neither a redirect nor a body fails or panics. The response was 200, with the error status written as text into the event stream.
- Read a subject field bound to a nested signal, as `signal:"chat.room"`, from the object Datastar sends, `{"chat":{"room":"lobby"}}`. The stream of a page handling such an event answered 400 on every connect.

#### Runtime and modules

- Percent-encode the characters a URL path cannot carry, such as `#`, `?` and `%`, in the file names that `assets.Path` and `href.Asset` turn into URLs. `assets.Path("a#b.css")` returned `/static/a#b.css`, which a browser requests as `/static/a`, and a `%` in a file name made the server answer 400.
- Label the HTTP metrics with the route when a middleware added with `datapages.WithMiddleware` passes on a new request, as `r.WithContext` returns when storing a CSP nonce. The request counter and the latency histogram labelled every request through such a middleware `<unmatched>`.
- Accept `&` and `'` in the URL passed to `datapages.WithDatastarJS`, as in `https://cdn.example.com/datastar.js?v=1&min=1`. RFC 3986 allows both, and the page writes the URL escaped. Such a URL was refused.
- Deliver events through the `natscore` broker in the order they were dispatched, as `inmem` does. Two events of different types dispatched one after the other often arrived in reverse order.
- End the SSE streams subscribed through the `natscore` broker when its NATS connection closes, which nats.go does after the last failed reconnect attempt: 60 attempts 2s apart with the defaults of `nats.Connect`. Open pages kept their streams and received no more events, without an error. They now reconnect, and the server logs each reconnect that fails on the closed connection.
- Return from `natskv.SessionManager.UserSessions` and `CloseAllUserSessions` the token the session's cookie carries, as `inmem` does. In v0.10.0 and v0.10.1, each call returned a new token for the same session, and an application that compares a listed token with `Session.Token()` to find the current session never found it. A session created before the upgrade, or under one of the `PreviousEncryptionKeys`, keeps a cookie that differs from its listed token until the user signs in again.
- Return from `natskv.SessionManager.UserSessions` an iterator that yields the sessions on every range, as `inmem` does. A second range yielded nothing, and an iterator never ranged kept a KV watcher subscribed until the context passed to `UserSessions` ended.
- Return an error from `natskv.SessionManager.UserSessions` and `DeleteExpired` when their context ends, NATS stalls for 5s or the connection closes during the call. `UserSessions` returned the sessions read by then as the complete list, and `DeleteExpired` returned nil with expired sessions left in the bucket.
- Report a token that can't be decrypted as `sessions.ErrSessionNotFound` in `natskv.SessionManager.Session`, as `inmem` does. This includes forged tokens and cookies encrypted with a key that is no longer in `PreviousEncryptionKeys`. Before, `errors.Is(err, sessions.ErrSessionNotFound)` was false for them.
- Send what net/http logs, such as a panic in a handler or a failed TLS handshake, to the logger set with `datapages.WithLogger`, at error level. It went to stderr as JSON at info level, and the metrics server of `datapages.WithPrometheus` wrote it as plain text. A server passed to `datapages.WithHTTPServer` keeps its own `ErrorLog` if it sets one.
- Register the offline service worker for the whole origin when `offline.Config.ScriptURL` is in a subdirectory, such as `/static/sw.js`. The browser limited such a worker to the pages under that directory. Other pages were not available offline, and their `datapages.PageCacheWriter` writes never reached the worker.
- Refuse an `offline.Config.ScriptURL` that is not a plain path, such as `sw.js` or `/sw.js?v=2`. The worker never loaded from those. `offline.WithServiceWorker` and `WithOffline` return an error, and `offline.Middleware` panics. Use a path that starts with `/` and holds only `/` and the unreserved characters of RFC 3986.
- Stop logging a panic, `response writer failed to flush`, with a stack trace when a visitor leaves a page while its SSE stream opens. A stream on the `natscore` broker waits for a round trip to NATS before it opens, which makes this more likely.

#### `datapages init`

- Shut down gracefully on SIGTERM in the `cmd/server/main.go` it writes. Docker, Kubernetes and systemd stop a process with SIGTERM, which previously ended the server without waiting for requests, SSE streams and `StreamClose` hooks. It does not rewrite an existing `main.go`: add `syscall.SIGTERM` to its `signal.NotifyContext` call.

### Security

#### Runtime and modules

- Prevent `natskv.SessionManager.CloseAllUserSessions` from returning nil while sessions of the user stay signed in. When its context ended during the call, as a request context does when the client disconnects, it closed only the sessions read by then. Whoever held a cookie of the others, such as an attacker the user meant to sign out, stayed signed in. A NATS stall of 5s during the call and a closed connection did the same. The end of the context no longer stops the call, and the other two return an error. Applications calling `CloseAllUserSessions` are affected in v0.1.0 through v0.10.1. Upgrade and redeploy. Until then, pass it `context.WithoutCancel(r.Context())`, which covers the client that disconnects.

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
