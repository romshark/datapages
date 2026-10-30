# Testing

Test domain logic with plain Go tests. Test the generated HTTP server with `net/http/httptest`. Every example on this page passes under `go test -race ./...`.

## The application under test

The examples use a notes board in the module `example.com/noteboard`:

- `PageIndex` at `/` renders the notes in `<ul id="notes">`.
- `POSTAdd` at `/add` takes the signal `text`, adds a note, dispatches `EventNotesChanged` and clears the signal with `sse.PatchSignals`.
- `POSTFilter` at `/filter` stores the signal `filter` in the per-tab state `StateIndex` and patches the filtered list.
- `OnNotesChanged` patches the list as the tab's filter selects it.
- `POSTClear` at `/clear` deletes every note. It takes the session and returns `datapages.ErrForbidden` to a guest.
- `(*App).POSTSignIn` at `/sign-in` signs in with the signal `name`.

The notes themselves live in the package `notes`, which knows nothing of Datapages.

## Test domain logic without HTTP

Keep rules and data access in packages that do not import Datapages, and test them directly:

```go title="notes/notes_test.go"
package notes_test

import (
	"slices"
	"testing"

	"example.com/noteboard/notes"
)

func TestFilter(t *testing.T) {
	all := []string{"buy milk", "buy bread", "call mom"}
	got := notes.Filter(all, "buy")
	want := []string{"buy milk", "buy bread"}
	if !slices.Equal(got, want) {
		t.Errorf("Filter(%q) = %q, want %q", "buy", got, want)
	}
}
```

These tests run without a server and name the rule that broke. The HTTP tests below check what the generated server adds, which is routing, signals, patches, events, state and sessions.

## Build the server in a test

The server that `datapages.NewServer` returns implements `http.Handler`. Build it in an external test package, as `cmd/server/main.go` does, with the in-memory broker and session manager:

```go title="app/app_test.go"
package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"

	"example.com/noteboard/app"
	"example.com/noteboard/app/datapagesgen"
	"example.com/noteboard/notes"
)

// newServer starts the generated server with in-memory modules.
func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	sessionManager := sessinmem.New[app.SessionData](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)
	s, err := datapages.NewServer[
		app.App,
		app.SessionData,
		datapages.EnablePrometheus,
		datapagesgen.Server,
	](
		&app.App{Notes: &notes.Store{}},
		inmem.New(0),
		datapages.WithSessionManager[app.SessionData](sessionManager),
		// Only ListenAndServe and ListenAndServeTLS open the metrics port.
		datapages.WithPrometheus(datapages.PrometheusConfig{Host: "127.0.0.1:0"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return srv
}
```

Use the same 4 type arguments as `cmd/server/main.go`. `datapages gen` reads `NewServer` calls in test files and rejects conflicting session types or metrics modes for 1 app. The scaffold uses `datapages.EnablePrometheus`, which requires `WithPrometheus`. The in-memory modules need neither NATS nor Docker. Each test builds its own `App` and server.

## Load a page

```go title="app/app_test.go"
func TestIndexPage(t *testing.T) {
	srv := newServer(t)
	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `<ul id="notes"></ul>`) {
		t.Errorf("no empty note list in:\n%s", body)
	}
}
```

The server compresses responses. `srv.Client()` uses a transport that requests gzip and decodes the response for the test. Send every request through `srv.Client()` or a copy of it. `httptest.Server.Close` closes idle connections in `http.DefaultTransport`. That can fail a concurrent test that uses `http.DefaultClient`.

## Send an action

An action that takes signals or `datapages.SSE` requires `Datastar-Request: true` and answers 406 without it. Datastar sends signals as JSON in the request body. `GET` and `DELETE` actions receive them in the `datastar` query parameter. These 2 helpers build and send a POST request:

```go title="app/app_test.go"
// newAction builds the request Datastar sends for a POST action:
// the signals as JSON in the body and the Datastar-Request header.
func newAction(t *testing.T, url string, signals any) *http.Request {
	t.Helper()
	body, err := json.Marshal(signals)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Content-Type", "application/json")
	return req
}

// do sends req and returns the status and the whole body.
func do(t *testing.T, c *http.Client, req *http.Request) (int, string) {
	t.Helper()
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body)
}

func TestAddClearsInput(t *testing.T) {
	srv := newServer(t)
	req := newAction(t, srv.URL+"/add/", map[string]string{"text": "buy milk"})
	status, body := do(t, srv.Client(), req)
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
	if !strings.Contains(body, `data: signals {"text":""}`) {
		t.Errorf("text signal not cleared:\n%s", body)
	}
}
```

An action that takes `sse datapages.SSE` answers with a `text/event-stream` body. The body ends when the handler returns. Each patch is an `event:` line followed by `data:` lines:

```text
event: datastar-patch-signals
data: signals {"text":""}
```

`sse.PatchElement` writes `event: datastar-patch-elements` and the HTML after `data: elements `. Assert on these lines.

When the app defines `App.RecoverError`, a failed Datastar action answers 200 with the events that `RecoverError` writes. Test those events. The CSRF and `Datastar-Request` checks run before the handler and keep their statuses 403 and 406.

## Test an event in another tab

A tab is a page load followed by its stream, a `GET` of the page path plus `_$/` with `Datastar-Request: true`. The stream of a page with per-tab state also needs the `Datapages-Instance` header of its page load and answers 409 without it. A stream does not end by itself, so a goroutine reads it while the test runs:

```go title="app/app_test.go"
// tab is 1 open browser tab: its page load and its stream.
type tab struct {
	instance string       // Datapages-Instance of the page load
	stream   lockedBuffer // what the stream carried so far
}

// openTab loads path and connects its stream, as a browser does.
// The stream closes when the test ends.
func openTab(t *testing.T, srv *httptest.Server, path string) *tab {
	t.Helper()
	page, err := srv.Client().Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	page.Body.Close()
	tb := &tab{instance: page.Header.Get("Datapages-Instance")}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+path+"_$/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Datapages-Instance", tb.instance)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status %d", resp.StatusCode)
	}
	go func() {
		defer resp.Body.Close()
		io.Copy(&tb.stream, resp.Body)
	}()
	return tb
}

// waitFor fails the test unless the stream carries s within 2 seconds.
func (tb *tab) waitFor(t *testing.T, s string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(tb.stream.String(), s) {
		if time.Now().After(deadline) {
			t.Fatalf("stream did not carry %q:\n%s", s, tb.stream.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// lockedBuffer is a bytes.Buffer that the stream goroutine writes
// while the test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestAddUpdatesOtherTabs(t *testing.T) {
	srv := newServer(t)
	other := openTab(t, srv, "/")

	req := newAction(t, srv.URL+"/add/", map[string]string{"text": "buy milk"})
	if status, body := do(t, srv.Client(), req); status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
	other.waitFor(t, `<ul id="notes"><li>buy milk</li></ul>`)
}
```

Mind the timing:

- Datapages subscribes a stream to the broker before it sends the response headers. Once `openTab` returns, the next dispatch reaches the stream, and the test needs no sleep.
- `StreamOpen` runs after the headers went out, and `StreamClose` after the client is gone. Poll for their effects as `waitFor` does.
- `t.Cleanup` runs its functions in reverse order. The `cancel` of `openTab` closes the stream before `srv.Close` waits for open requests. Without it, `Close` blocks until the test times out.
- That a patch did not arrive can only be shown by waiting. Assert the exact patch that must arrive instead, as the next test does.

## Test per-tab state

Copy `Datapages-Instance` from the page response to the stream and to every action of that tab, as the page does in the browser:

```go title="app/app_test.go"
func TestFilterIsPerTab(t *testing.T) {
	srv := newServer(t)
	filtered, plain := openTab(t, srv, "/"), openTab(t, srv, "/")

	req := newAction(t, srv.URL+"/filter/", map[string]string{"filter": "milk"})
	req.Header.Set("Datapages-Instance", filtered.instance)
	if status, body := do(t, srv.Client(), req); status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}

	for _, text := range []string{"buy bread", "buy milk"} {
		req := newAction(t, srv.URL+"/add/", map[string]string{"text": text})
		if status, body := do(t, srv.Client(), req); status != http.StatusOK {
			t.Fatalf("status %d: %s", status, body)
		}
	}
	filtered.waitFor(t, `<ul id="notes"><li>buy milk</li></ul>`)
	plain.waitFor(t, `<ul id="notes"><li>buy bread</li><li>buy milk</li></ul>`)
}
```

Both tabs receive both events, and each renders the list with its own filter. A stateful action whose instance has no open stream answers `409 Conflict` with `Datapages-Retry: reconnect`, which also happens when the test sends the action before `openTab`.

## Test a signed-in client

A signed-in client keeps the session cookie in a cookie jar and sends the CSRF token in the header `X-CSRF-Token` with every `POST`, `PUT`, `PATCH` and `DELETE` action:

```go title="app/app_test.go"
// csrfToken finds the token in the script that Datapages writes
// into every page rendered for a session.
var csrfToken = regexp.MustCompile(`"X-CSRF-Token",'([^']+)'`)

// signIn returns a client with a session for name in its cookie jar,
// and the CSRF token of that session.
func signIn(t *testing.T, srv *httptest.Server, name string) (*http.Client, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := *srv.Client() // a copy: Client returns 1 client for all callers
	c.Jar = jar
	req := newAction(t, srv.URL+"/sign-in/", map[string]string{"name": name})
	if status, body := do(t, &c, req); status != http.StatusOK {
		t.Fatalf("sign-in status %d: %s", status, body)
	}

	req, err = http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, page := do(t, &c, req)
	m := csrfToken.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no CSRF token in:\n%s", page)
	}
	return &c, m[1]
}

func TestClearNeedsCSRFToken(t *testing.T) {
	srv := newServer(t)
	c, token := signIn(t, srv, "alice")

	req := newAction(t, srv.URL+"/clear/", struct{}{})
	if status, _ := do(t, c, req); status != http.StatusForbidden {
		t.Errorf("without token: status %d, want 403", status)
	}

	req = newAction(t, srv.URL+"/clear/", struct{}{})
	req.Header.Set("X-CSRF-Token", token)
	if status, body := do(t, c, req); status != http.StatusOK {
		t.Errorf("with token: status %d: %s", status, body)
	}
}
```

The sign-in request needs no token, because it carries no session cookie yet. A sign-in that returns a redirect answers a Datastar request with status 200 and a script. The session cookie is `Secure`, and `net/http/cookiejar` sends it over plain HTTP to the loopback address of `httptest`. `QUERY` actions and page loads need no token.

## Lint and CI

[`datapages lint`](cli.md#datapages-lint) checks the application model and the `.templ` files without writing files, and exits with status 1 on an error. It reports hardcoded URLs and actions of another page among others. `datapages gen` applies the same checks before it generates.

`datapages init` writes `.github/workflows/ci.yml` unless `--ci none` is set. The workflow installs Go, templ `v0.3.1100` and the `datapages` CLI of the version in `go.mod`, then:

1. runs `templ generate`, and with committed generated code fails when that changes a committed `.go` file,
2. runs `datapages gen` and fails on an error, and with committed generated code on any change to a committed file,
3. runs `go test ./...`,
4. builds `./cmd/server/`.

Step 2 fails on every error `datapages lint` reports. The tests read streams on goroutines, so change step 3 to `go test -race ./...` to run them under the race detector.

## See also

- [Linting](../SPECIFICATION.md#linting): the checks of `datapages lint`.
- [CLI](cli.md): the commands and their flags.
- [Slow and failing networks](network.md): try the app by hand on a slow network, with failing requests and with the server unreachable.
- [Events](events.md), [Per-tab state](state.md) and [Sessions and auth](sessions.md): the behavior these tests check.
- [Pages](../SPECIFICATION.md#pages): which requests need `Datastar-Request: true`.
- [`datapagestest`](../datapagestest/): the HTTP test client for Datapages apps, with visitors, tabs, actions, SSE streams and CSRF tokens. [`internal/acceptance/datapagestest`](../internal/acceptance/datapagestest/datapagestest_test.go) runs it against a generated server.
