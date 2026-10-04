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

#### Runtime and modules

- Reload open pages when a deployment changes the build. Set `datapages.WithBuildID` when replicas run different binaries of one release. A `Content-Security-Policy` must allow the inline build script.

### Changed

#### `datapages lint` and `datapages gen`

- Reject actions, `OnXXX` handlers, `StreamOpen`, `StreamClose` and `State[T]` on a page whose route ends in `{$}`, as `// PageUser is /user/{name}/{$}`. A `{$}` at the end of a page route declares a page that serves only `GET`. An action below such a route got an error saying that it was not under its page, with a suggested route that net/http rejects. Remove `{$}` from the route of such a page: it keeps its URL.
- Read an exported method on `App` named `QUERY` followed by an uppercase letter, such as `QUERYStats`, as an action. Such a method is rejected when it's a helper rather than an action: rename it.

#### Generated code

Run `datapages gen` to apply these.

- Reload a page when the session of its SSE stream closes or expires. A page with only user-addressed events also reloads when its stream reconnects after the session ended. The page previously kept showing the old session's content without updates.
- Render `PageError404` with status 404 for a page load whose handler returns `datapages.ErrNotFound`, as for a URL that no route matches. For such a page load, `PageError404.GET` receives the request's query and signals and a zero path. An app without `PageError500` previously answered with the status text.
- Answer 406 Not Acceptable to a request without `Datastar-Request: true`, such as a plain form submission, on an action declared on `App` that takes `datapages.PageCacheWriter`, as on a page. Such an action that redirects answered a plain form with an HTTP redirect and dropped the queued writes, such as `ClearAll`. Submit those forms through a Datastar action.

#### Runtime and modules

- Label requests with the `QUERY` method as `QUERY` in the `method` label of the HTTP metrics. They were counted as `<other>`.
- Refuse a URL passed to `datapages.WithDatastarJS` that is invalid according to RFC 3986.

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

#### Runtime and modules

- Percent-encode the characters a URL path cannot carry, such as `#`, `?` and `%`, in the file names that `assets.Path` and `href.Asset` turn into URLs. `assets.Path("a#b.css")` returned `/static/a#b.css`, which a browser requests as `/static/a`, and a `%` in a file name made the server answer 400.
- Label the HTTP metrics with the route when a middleware added with `datapages.WithMiddleware` passes on a new request, as `r.WithContext` returns when storing a CSP nonce. The request counter and the latency histogram labelled every request through such a middleware `<unmatched>`.
- Accept `&` and `'` in the URL passed to `datapages.WithDatastarJS`, as in `https://cdn.example.com/datastar.js?v=1&min=1`. RFC 3986 allows both, and the page writes the URL escaped. Such a URL was refused.

#### `datapages init`

- Shut down gracefully on SIGTERM in the `cmd/server/main.go` it writes. Docker, Kubernetes and systemd stop a process with SIGTERM, which previously ended the server without waiting for requests, SSE streams and `StreamClose` hooks. It does not rewrite an existing `main.go`: add `syscall.SIGTERM` to its `signal.NotifyContext` call.

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
