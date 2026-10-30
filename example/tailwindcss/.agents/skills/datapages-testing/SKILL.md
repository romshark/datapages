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

Pass `datapagestest.WithSessionCookie` to `New` for a server whose `SessionsConfig` renames the session cookie, which `v.SetSessionToken` sets. `datapagestest.WithTimeout` changes the 5s limit of every request and wait.

## Visitors, tabs and streams

```go
v := srv.Visitor(t)
tab := v.Load(t, href.PageIndex())
st := tab.Connect(t, tab.Page().PublicSignals(t))
tab.Do(t, http.MethodPost, "/add/", map[string]any{"title": "Buy milk"}).
	RequireStatus(t, http.StatusOK)
ev := st.WaitFor(t, datapagestest.TextContains("rows", "Buy milk"))
```

- A `Visitor` is one browser with its own cookie jar. Create one per user.
- `v.Get` loads a page the way a navigation does, with `Sec-Fetch-Dest: document`, and follows no redirect. The error of a GET action that answers with a file renders as the error page only for such a request. `v.Load` also fails unless the page answers 200, and returns a `Tab`.
- A tab keeps the `Datapages-Instance` header of its page load and sends it on its stream and actions. Load the page again for a second tab. A second `Connect` on the same tab is the same tab and takes over its state.
- `tab.Connect` opens the page's stream at the page path plus `_$/` and fails unless the answer is `text/event-stream`. It follows the 303 that sends a signed-out visitor to `_$/anon/`. The stream closes when the test ends. `st.Close()` closes it earlier.
- `tab.Do` sends an action the way Datastar does: with `Datastar-Request: true`, and with the JSON signals in the body of a POST, PUT, PATCH or QUERY and in the `datastar` query parameter of a GET or DELETE. A nil signals sends `{}`. Keys missing from the JSON decode as zero values.
- `resp.RequireStatus(t, status)` fails the test unless the response has status, naming the request and quoting the body, and returns the response. Check the status of every action whose effect the test waits for on a stream: a refused action otherwise fails later, at the timeout of the wait.
- `tab.DoForm` sends the fields of an action built with `action.WithContentType(action.ContentTypeForm)`.
- `tab.NewRequest` builds the tab's Datastar request for any other case, such as a request without its CSRF token. Send it with `v.Send`.
- `v.Send` and every action fail the test when the response doesn't complete within the timeout, and `tab.Connect` when the response header doesn't arrive. `v.Client()` sends a request whose response `v.Send` cannot read whole, such as a stream whose status or headers the test checks, without a timeout. Close the response body.
- `datapagestest.WithHTTPServer(func(s *http.Server) {...})` configures the server that serves the app, such as the `ReadTimeout` the server command sets. The in-memory connections honor its deadlines.

Build page URLs with the generated `href` package, such as `href.PageIndex()`. Send each action to the route in its doc comment. That request does not show that the page calls the action: also assert that the triggering element's `data-on:*` attribute contains the call the `action` helper writes, such as `@post('/filter/'`. For an action whose query parameters the `action` helper encodes, `datapagestest.ParseAction(t, expr)` returns the method and URL of the call it writes.

Send the browser's signals, including the visitor's changes. `tab.Page().Signals(t)` returns the signals the page's `data-signals` attributes declare, named and decoded the way Datastar does, and fails on a value that is not a literal. By default Datastar sends every public signal with each action and stream connection and leaves out each signal with a name on its path that starts with an underscore. `tab.Page().PublicSignals(t)` returns what it sends. Action options override this:

- `action.WithFilterSignals` sends the signals its patterns select. An empty exclude pattern keeps private signals out.
- `action.WithPayload` sends its payload in place of the signals.
- `action.WithContentType(action.ContentTypeForm)` sends the form's fields and no signals.

## Responses

Assert action output in its response and `On` output in the receiving tab's stream. `resp.Events(t)` parses the event stream an action with an `sse` parameter answers with. `resp.Event(t, m)` returns the first of them that a stream's matcher `m` matches and fails the test when none does, as in `resp.Event(t, datapagestest.TextContains("out", "done"))`. Errors from a `GET`, action or `StreamOpen` use the first matching rule:

1. A Datastar request, when the app defines `RecoverError`: 200 with the events `RecoverError` writes.
2. A Datastar request whose response is already an event stream: 200 with the events written before the error. Streams, actions with an `sse` parameter and `pageCache` actions that return neither a body nor a redirect start the event stream before the handler runs.
3. A request without `Datastar-Request`, when the app defines `PageError500`: that page with status 500, whatever the sentinel.
4. Any other request: 400 for `ErrBadRequest`, 403 for `ErrForbidden`, 404 for `ErrNotFound`, 409 for `ErrConflict` and 500 for any other error.

An error returned from an `On` handler is only logged. A request the server rejects before the handler runs keeps its status, such as 400 for undecodable signals, 403 for a missing CSRF token or 406 for a missing `Datastar-Request`.

## HTML

Assert the HTML the server sends. `resp.Get(t, q)` in a page and `ev.Get(t, q)` in a patch return the element a query selects and fail unless exactly 1 matches. A patch is parsed inside a `<template>` the way Datastar parses it, which keeps a `<tr>` or `<td>` at its root. Prefer the first query that fits:

1. `datapagestest.Role(role, name)`: the ARIA role, implicit as `button` for `<button>` and `link` for `<a href>`, or explicit, and the accessible name. An empty name matches any.
2. `datapagestest.Label(text)`: the form control a `<label>`, `aria-label` or `aria-labelledby` names.
3. `datapagestest.HasText(text)`: the innermost element whose whole text is text.
4. `datapagestest.ID(id)`, `datapagestest.Tag(name)` and `datapagestest.Where(what, f)`: what has no role, such as a patch target or a custom element.

`q.In(container)` narrows a query to the inside of another, as `Role("button", "Delete").In(Role("listitem", ""))`, and `q.Attr(key, value)` and `q.HasText(text)` to an attribute and a whole text. `Role`, `Label` and `HasText` skip what the HTML hides with the `hidden` attribute, `aria-hidden`, `display: none` in a `style` attribute or a closed `<dialog>` around it. Datastar attributes such as `data-show` are not evaluated. A failed query prints the accessibility tree it searched.

An `Element` reports `Text()`, `Attr(key)`, `Value()` and `Checked()` as the HTML sets them. A value Datastar binds to a signal is in `resp.Signals(t)` instead. `el.Get` and `el.All` query inside it, and `resp.All(q)` and `ev.All(q)` return every match.

`el.RequireTree(t, want)` compares the accessibility tree inside an element with a `datapagestest.Tree`. A `Node` holds `Role`, `Name`, `Value`, `Text`, `URL`, `Level`, `Checked`, `Disabled`, `Expanded`, `Selected`, `Pressed` and `Children`. The comparison is exact, so an extra row fails. Elements without a role, such as `div` and `span`, and hidden elements are not nodes. Text beside elements is a node with the role `text`, and a node that contains only text has it in `Text`, left out when it equals `Name`. On a mismatch the failure prints the difference and the actual tree as Go. Compute expected values with Go, as `URL: href.PagePost(slug)`.

A query that cannot find a control by its role and name points at a gap a screen reader user meets too. Give controls native elements, labels and accessible names rather than attributes for the test. A substring of the whole page can match the wrong element and pass. Raw-markup comparisons break on attribute order, escaping or whitespace changes that preserve browser-visible behavior.

## Streams

- `st.WaitFor(t, m)` waits for the first event after the stream's cursor that `m` matches and moves the cursor past it. Match the expected content with `datapagestest.TextContains`, `Patched`, `Signal` or `Func`, not any event: `StreamOpen` output and earlier events are on the same stream. A message after the matcher, as with `require`, opens the failure message and tells two streams of one page apart.
- `st.Absent(t, prohibited, anchor)` waits for the anchor and fails when a prohibited event came before it. Publish the anchor after the prohibited events, on a subject the checked stream receives. An anchor seen on another stream proves nothing about this one. Every `messaging.Broker` delivers messages published one after the other in that order, whatever their subjects. With `natscore` this holds for the messages one NATS connection publishes, such as the server's.
- The stream subscribes before sending its response header. Events dispatched after `Connect` returns therefore reach it. With `natscore`, this guarantee covers only the server's NATS connection because other connections can publish before the subscription is registered.
- `ev.Signals(t)` decodes a signal patch to raw values: a signal the patch deletes is `null`, and a signal it leaves out has no key. Decode `ev.Data["signals"]` into a struct for ordinary comparisons.
- `ev.Script()` returns the script of an event `sse.ExecuteScript` sends, and false for any other event.
- `st.WaitEnd(t)` waits for the server to end the stream, and `st.Ended()` reports whether it has ended. The server ends a stream when its session closes or expires, after it sends a script that reloads the page, which `datapagestest.Reload()` matches, and when `StreamOpen` fails. `ev.Retry` is the reconnection delay the last event of such a stream sets.
- `st.Path()` is the route that answered: the page path plus `_$/`, or `_$/anon/` after the redirect of a signed-out visitor.

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

- Every wait of the package ends at a deadline: a stream's wait as soon as an event arrives, `srv.WaitFor` by polling its condition. Do not add a fixed sleep: it can finish too early and cannot prove absence. `srv.WaitFor` waits for an effect no stream carries.
- `datapagestest` serves the app over in-memory connections, which a `testing/synctest` bubble can wait on. Inside `synctest.Test`, `time.Sleep` advances the clock that the server's timers, the `http.Server` deadlines and the app's `time.Now` read, such as a session's expiry, and a wait of an hour passes at once. Run a test that would otherwise wait on the real clock, such as for a timeout, a rate limit or an expiry, in a bubble. Build the server and the broker inside the bubble. `t.Run` panics inside a bubble, and a broker on a socket, such as `natscore`, cannot join one. Outside a bubble, advance a clock injected into the app and assert exact times.

## Time zones and random values

- Go initializes `time.Local` from `TZ` on first use; a later `t.Setenv("TZ", ...)` has no effect. Inject the location or use UTC, with a fixed location in tests.
- Read random `Datapages-Instance`, session tokens, CSRF tokens and app-generated IDs from their responses. Assert properties such as later reuse or inequality, and inject the generator when a test requires a known value.

## Sessions

Sign in through the app's action with `tab.Do`; a guest sends it without a CSRF token. The visitor's jar keeps the session cookie. A tab sends the `X-CSRF-Token` its page's script carries, as the browser does: a page rendered for a signed-in visitor carries one, and a page rendered for a guest carries none, also after the sign-in. Load a page again after signing in, as the browser does when the sign-in redirects. An action whose response carries the script, such as a sign-in that answers with a document, gives its tab the token. For an app whose sign-in a test cannot send, such as one through an identity provider, create the session with the session manager's `CreateSession` and pass its token to `v.SetSessionToken`.

To test the CSRF check, build the request with `tab.NewRequest`, delete its `X-CSRF-Token` header and send it with `v.Send`. The server answers 403.

`v.Cookie(name)` returns the name and value the jar keeps. `resp.Cookies()` parses the cookies a response sets with their attributes, such as `Secure`, `HttpOnly` and `MaxAge`.

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

<!-- written by datapages sha256:d78d5dad9433b161 -->
