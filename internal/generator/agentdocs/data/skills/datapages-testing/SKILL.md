---
name: datapages-testing
description: >-
  Test a Datapages app over HTTP with the datapagestest package. Build the server
  in a test, load pages, send Datastar actions, read SSE streams and test per-tab
  state, events, sessions, CSRF and time. Use when writing, fixing or reviewing
  tests of pages, actions, stream hooks or event handlers, or when a Datapages
  test is flaky.
---

# Testing

Read `datapages` first for the build loop, rules and naming conventions.

Test the app over HTTP with `github.com/romshark/datapages/datapagestest`. It sends the requests Datastar sends and reads the pages, events and redirects the server answers with. Direct handler calls bypass routing, signal decoding, per-tab state and rendering. Use them only for independent logic such as parsing or formatting. `go build ./...` compiles the code but sends no requests.

After changing the app model, run `datapages gen`; after changing a `.templ` file, run `templ generate`. Then run `go test -race ./...`. Under `datapages watch`, let the watcher regenerate because a separate `templ generate` prevents the development server from rendering.

## The server

Build the server in the app's external test package, `package app_test`, with the same four `NewServer` type arguments and the same options as the server command, and pass it to `datapagestest.New`. `datapages gen` reads every `NewServer` call. A test server without the command's middleware, session, CSRF or body-limit options answers differently.

```go
func newServer(t *testing.T, a *app.App, broker messaging.Broker) *datapagestest.Server {
	t.Helper()
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, broker, datapages.WithAssets(app.StaticFS, false))
	if err != nil {
		t.Fatal(err)
	}
	return datapagestest.New(t, s)
}
```

Build `a` with the app's constructor and test dependencies such as a controllable clock or in-memory store. A zero `app.App` lacks what the constructor sets and can panic in its handlers. Keep the dependencies and the broker in the test to derive expected responses and to publish events.

Pass `inmem.New(n)` as the broker. It buffers `n` messages per subscription and drops messages when full. Set `n` above the number of events a test can publish before its stream reads them.

Pass `datapagestest.WithSessionCookie` or `datapagestest.WithCSRFTokens` to `New` for a server whose `SessionsConfig` renames the session cookie or whose `CSRFConfig` replaces the token writer. `datapagestest.WithTimeout` changes the 5s limit of every wait.

## Visitors, tabs and streams

```go
v := srv.Visitor(t)
tab := v.Load(t, href.PageIndex())
st := tab.Connect(t, tab.Page().Signals(t))
tab.Do(t, http.MethodPost, "/add/", map[string]any{"title": "Buy milk"})
ev := st.WaitFor(t, datapagestest.TextContains("rows", "Buy milk"))
```

- A `Visitor` is one browser with its own cookie jar. Create one per user.
- `v.Get` loads a page the way a navigation does and follows no redirect. `v.Load` also fails unless the page answers 200, and returns a `Tab`.
- A tab keeps the `Datapages-Instance` header of its page load and sends it on its stream and actions. Load the page again for a second tab. A second `Connect` on the same tab is the same tab and takes over its state.
- `tab.Connect` opens the page's stream at the page path plus `_$/` and fails unless the answer is `text/event-stream`. It follows the 303 that sends a signed-out visitor to `_$/anon/`. The stream closes when the test ends. `st.Close()` closes it earlier.
- `tab.Do` sends an action the way Datastar does: with `Datastar-Request: true`, and with the JSON signals in the body of a POST, PUT or PATCH and in the `datastar` query parameter of a DELETE. A nil signals sends `{}`. Keys missing from the JSON decode as zero values.
- `tab.DoForm` sends the fields of an action built with `action.WithContentType(action.ContentTypeForm)`.
- `tab.NewRequest` builds the tab's Datastar request for any other case, such as a request without its CSRF token. Send it with `v.Send`.

Build page URLs with the generated `href` package, such as `href.PageIndex()`. Send each action to the route in its doc comment. That request does not show that the page calls the action: also assert that the triggering element's `data-on:*` attribute contains the call the `action` helper writes, such as `@post('/filter/'`.

Send the browser's signals, including the visitor's changes. `tab.Page().Signals(t)` returns the signals the page's `data-signals` attributes declare, named and decoded the way Datastar does, and fails on a value that is not a literal. By default Datastar sends every public signal with each action and stream connection and omits names that start with an underscore. Action options override this:

- `action.WithFilterSignals` sends the signals its patterns select. An empty exclude pattern keeps private signals out.
- `action.WithPayload` sends its payload in place of the signals.
- `action.WithContentType(action.ContentTypeForm)` sends the form's fields and no signals.

## Responses

Assert action output in its response and `On` output in the receiving tab's stream. `resp.Events(t)` parses the event stream an action with an `sse` parameter answers with. Errors from a `GET`, action or `StreamOpen` use the first matching rule:

1. A Datastar request, when the app defines `RecoverError`: 200 with the events `RecoverError` writes.
2. A Datastar request whose response is already an event stream: 200 with the events written before the error. Streams, actions with an `sse` parameter and `pageCache` actions that return neither a body nor a redirect start the event stream before the handler runs.
3. A request without `Datastar-Request`, when the app defines `PageError500`: that page with status 500, whatever the sentinel.
4. Any other request: 400 for `ErrBadRequest`, 403 for `ErrForbidden`, 404 for `ErrNotFound`, 409 for `ErrConflict` and 500 for any other error.

An error returned from an `On` handler is only logged. A request the server rejects before the handler runs keeps its status, such as 400 for undecodable signals, 403 for a missing CSRF token or 406 for a missing `Datastar-Request`.

## HTML

Find the element by `id` and assert its text or relevant attribute: `resp.ElementByID(t, id)` in a page and `ev.ElementByID(t, id)` in a patch, then `datapagestest.Text(n)` or `datapagestest.Attr(n, key)`. A patch is parsed inside a `<template>` the way Datastar parses it, which keeps a `<tr>` or `<td>` at its root. A substring of the whole page can match the wrong element and pass. Raw-markup comparisons break on attribute order, escaping or whitespace changes that preserve browser-visible behavior.

## Streams

- `st.WaitFor(t, m)` waits for the first event after the stream's cursor that `m` matches and moves the cursor past it. Match the expected content with `datapagestest.TextContains`, `Element`, `Signal` or `Func`, not any event: `StreamOpen` output and earlier events are on the same stream.
- `st.Absent(t, prohibited, anchor)` waits for the anchor and fails when a prohibited event came before it. Publish the anchor after the prohibited events, on a subject the checked stream receives. An anchor seen on another stream proves nothing about this one. Use `inmem`, which delivers a stream's messages in publication order; `natscore` preserves the order only within one subject.
- The stream subscribes before sending its response header. Events dispatched after `Connect` returns therefore reach it. With `natscore`, this guarantee covers only the server's NATS connection because other connections can publish before the subscription is registered.
- `ev.Signals(t)` decodes a signal patch to raw values: a signal the patch deletes is `null`, and a signal it leaves out has no key. Decode `ev.Data["signals"]` into a struct for ordinary comparisons.

## Redirects

A handler redirects by returning `datapages.Redirect` or calling `sse.Redirect`. `resp.Redirect(t)` returns the target in whichever form the request received it:

- A plain request, page load or form post gets an HTTP redirect with the target in `Location`. The status defaults to 302; `Redirect.Status` can set 301, 303, 307 or 308.
- Before SSE starts, a Datastar request gets a `text/javascript` script that sets `window.location`.
- After an action with an `sse` parameter writes the response header, and for every `sse.Redirect`, the redirect is a `datastar-patch-elements` event. `ev.Redirect()` reads it from a stream's event.

## Events

An action that dispatches an event also tests its `On` handler. Test code that publishes outside a handler by running it against the test's broker. To test only the receiving pages, publish with `datapagestest.Publish(t, broker, subject, event)`. Use `datapagesgen.EvSubj<Name>` as the subject of an event without subject fields. For an event with subject fields, use `datapagestest.Subject(datapagesgen.EvPrefix<Name>, values...)`, which escapes the values the way the dispatcher does. `EvSubj<Name>` of such an event is a wildcard subscription pattern, not a publication subject. An event with a `datapages.SubjectStateID` field has no subject a test can build: dispatch it from a handler.

Test recipient and non-recipient streams. Prove each expected absence with `st.Absent`:

- An event addressed through a `datapages.SubjectStateID` field reaches only that tab. Load the page again for a second tab and prove that its stream receives nothing.
- A subject field tagged `signal:"name"` uses the stream's connection signal. Connect two tabs with different values and prove that the event reaches only the matching tab.
- An event addressed to a user reaches only that user's streams. See [Sessions](#sessions).

`StreamClose` cannot write to its closed stream. After `st.Close()`, wait for an external effect: an event on another tab, or a store update with `srv.WaitFor(t, what, cond)`.

## State

A page load creates `Datapages-Instance` without state; the stream connection allocates the tab's state. A stateful action before connection or after closure returns 409 with `Datapages-Retry: reconnect`, which `resp.Retry()` reads. Reopening the same instance starts with zero state. Test that reconnection restores required state from the URL or signals.

## Waiting

- Every wait of the package polls until a deadline. Do not add a fixed sleep: it can finish too early and cannot prove absence. `srv.WaitFor` waits for an effect no stream carries.
- `datapagestest` serves the app over in-memory connections, which a `testing/synctest` bubble can wait on. Inside `synctest.Test`, `time.Sleep` advances the clock that the server's timers and the app's `time.Now` read, such as a session's expiry. Outside a bubble, advance a clock injected into the app and assert exact times.

## Time zones and random values

- Go initializes `time.Local` from `TZ` on first use; a later `t.Setenv("TZ", ...)` has no effect. Inject the location or use UTC, with a fixed location in tests.
- Read random `Datapages-Instance`, session tokens, CSRF tokens and app-generated IDs from their responses. Assert properties such as later reuse or inequality, and inject the generator when a test requires a known value.

## Sessions

Sign in through the app's action with `tab.Do`; a guest sends it without a CSRF token. The visitor's jar keeps the session cookie, and the visitor's later actions carry the `X-CSRF-Token` the server derives from it. For an app whose sign-in a test cannot send, such as one through an identity provider, create the session with the session manager's `CreateSession` and pass its token to `v.SetSessionToken`.

To test the CSRF check, build the request with `tab.NewRequest`, delete its `X-CSRF-Token` header and send it with `v.Send`. The server answers 403.

Test two visitors: one user's updates must not reach the other's stream, and a cross-user action must return `ErrForbidden` without changing data. Assert the response specified in [Responses](#responses). Without `RecoverError`, an action with an `sse` parameter returns 200 with an empty stream.

## Assertions

- Derive exact expected signals, element text, attributes and status from the requirement or handler doc comment, not the current output. Ask when the requirement is unclear.
- Assert absence where an incorrect or duplicate effect is possible: a removed item's signal is `null`, another tab receives nothing or a list contains an item once.
- Call the package and assert on the test goroutine: `datapagestest` fails the test with `t.Fatalf`.
- Check the fixture count before a loop; an empty set runs no assertions. Include the expected value, actual value and request in each failure message.

## Proving a test

- For a bug, reproduce it with a failing test before the fix, then confirm that it passes. For other behavior, introduce a defect and confirm that the test fails; a passing test does not cover it.
- Run with `-count=1`. A `(cached)` result did not execute the test again. `[no tests to run]` means that the `-run` pattern matched no tests.
- Classify a failure as a bug or an incorrect requirement before changing the test. Do not loosen an assertion, add `t.Skip`, widen a tolerance or delete a case only to make it pass.
