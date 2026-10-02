# Changelog

This file records the notable changes of each release in the format of
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The release
workflow publishes the section of a version as its GitHub release notes.
Releases up to v0.10.0 have their notes on
[GitHub Releases](https://github.com/romshark/datapages/releases) only.

## [Unreleased]

### Added

- Reload open pages when a deployment changes the build. Set `datapages.WithBuildID` when replicas run different binaries of one release. A `Content-Security-Policy` must allow the inline build script.

### Changed

- Reload a page when the session of its SSE stream closes or expires. A page with only user-addressed events also reloads when its stream reconnects after the session ended. The page previously kept showing the old session's content without updates. Run `datapages gen`.
- Render `PageError404` with status 404 for a page load whose handler returns `datapages.ErrNotFound`, as for a URL that no route matches. For such a page load, `PageError404.GET` receives the request's query and signals and a zero path. An app without `PageError500` previously answered with the status text. Run `datapages gen`.

### Fixed

- Reconnect a page's SSE stream whenever it ends, without a retry limit. Some endings, such as a graceful shutdown, previously left the page without live updates. Run `datapages gen`.
- Write the CSRF script into every request-specific page and action response whose actions carry a session cookie, including documents whose handlers do not accept a session. This prevents actions submitted from those documents from returning 403. `PageOffline` and page-cache entries omit the script because visitors share them. Run `datapages gen`.
- Shut down gracefully on SIGTERM in the `cmd/server/main.go` that `datapages init` writes. Docker, Kubernetes and systemd stop a process with SIGTERM, which previously ended the server without waiting for requests, SSE streams and `StreamClose` hooks. `datapages init` does not rewrite an existing `main.go`: add `syscall.SIGTERM` to its `signal.NotifyContext` call.
- Stop `datapages gen` from writing code that does not compile when `App.Head` takes the session before the `*http.Request` in an app that uses `datapages.PageCacheWriter`. Run `datapages gen`.
- Stop `datapages gen` from writing code that does not compile when the type argument of `datapages.Path`, `datapages.Query` or `datapages.Signals` is an alias of a defined type, as in `type Filter = SearchQuery`. Run `datapages gen`.
- Stop `datapages gen` from writing code that does not compile when the type argument of `datapages.Path`, `datapages.Query` or `datapages.Signals` is a struct type literal with an embedded field, such as `time.Time`. Run `datapages gen`.
- Stop `datapages gen` from writing code that does not compile when the session data type or a field type of `datapages.Path`, `datapages.Query` or `datapages.Signals` is an alias declared in a package with the same name as one the generated code imports, such as `stream`. Run `datapages gen`.
- Reject a page that embeds two types declaring one action at the same depth, or that reaches one abstract page through two embedded types at the same depth. The generated code for such a page did not compile: Go reported an ambiguous selector. Declare the handler on the page, which shadows the embedded ones.
- Stop `datapages gen` from writing code that does not compile when `PageError404.GET` takes a path, query, signals or dispatcher parameter. For a URL that no route matches, the handler receives that URL's query and signals and a zero path. Run `datapages gen`.
- Reject an `OnXXX` handler that takes a second `datapages.SSE`, session or `datapages.StreamID` parameter, as every other handler does. A field with two names, such as `sse, sse2 datapages.SSE`, declares two parameters: `datapages gen` read it as one and wrote a call that did not compile.
- Reject an `OnXXX` handler that takes its event by pointer, as in `event *EventPing`. The generated code for it did not compile. Take the event by value.
- Reject a route containing `"` or `\` in `datapages lint` and `datapages gen`. For such a route, `datapages gen` failed with a Go syntax error, or the generated server matched a different path when the backslash formed a Go escape such as `\u00e9`. Write the characters as `%22` and `%5C`, which `net/http` decodes before matching the route.
- Reject a route containing a backtick in `datapages lint` and `datapages gen`. On a page with a stream or a query field that reflects a signal, `datapages gen` failed with a Go syntax error. Write it as `%60`, which `net/http` decodes before matching the route.
- Stop `datapages gen` from writing code that does not compile when a path, query or signals field type comes from a package named `path`, `query`, `signals` or after another variable the generated handlers declare. Run `datapages gen`.
- Reject an unexported defined type as the type argument of `datapages.Path`, `datapages.Query` or `datapages.Signals`, including one behind an exported alias. The generated code for such a handler did not compile. Export the type.
- Answer a page load whose handler returns `datapages.ErrBadRequest`, `datapages.ErrForbidden`, `datapages.ErrNotFound` or `datapages.ErrConflict` with that status in an app that defines `PageError500`, as every app created by `datapages init` does. Such a page load got `PageError500` with status 500, and no page could answer 404. `PageError500` now renders for status 500 only. Other statuses get their status text, except 404 in an app that defines `PageError404`. Run `datapages gen`.
- Keep the action of an embedded type when the page, or the embedded type, declares an action with the same suffix for another HTTP method, such as an embedded `PUTSave` next to `POSTSave`. `datapages gen` dropped the embedded action: the server did not route it and the `action` package had no builder for it. Run `datapages gen`.
- Keep the SSE streams of a page with both public and user-addressed events open while the tab is hidden when its `GET` returns `enableBackgroundStreaming` set to `true`, for signed-in visitors and guests alike. Such a page dropped its reload on visibility but still closed its stream, which left it without the events published while the tab was hidden. Run `datapages gen`.

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
