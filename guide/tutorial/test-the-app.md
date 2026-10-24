# Test the app

The server that `datapages.NewServer` returns implements `http.Handler`, which `net/http/httptest` can serve without NATS or Docker. Create `app/app_test.go`:

```go title="app/app_test.go"
package app_test

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/modules/messaging"
	msginmem "github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"

	"example.com/shoplist/app"
	"example.com/shoplist/app/datapagesgen"
)

// csrfToken finds the token in the script that adds the X-CSRF-Token header
// to the actions of a page rendered for a session.
var csrfToken = regexp.MustCompile(`"X-CSRF-Token",'([^']+)'`)

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	s, err := datapages.NewServer[
		app.App,
		app.SessionData,
		datapages.EnablePrometheus,
		datapagesgen.Server,
	](
		&app.App{},
		msginmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithSessionManager[app.SessionData](sessinmem.New[app.SessionData](
			sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
		)),
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

// TestAddItem tests that a guest cannot add an item and a signed-in user can.
func TestAddItem(t *testing.T) {
	srv := newServer(t)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Transport: srv.Client().Transport}

	post(t, client, srv.URL+"/items", "", `{"name":"Milk"}`)
	if strings.Contains(get(t, client, srv.URL+"/"), "Milk") {
		t.Fatal("a guest added an item")
	}

	post(t, client, srv.URL+"/sign-in", "", `{"user":"Alice"}`)
	m := csrfToken.FindStringSubmatch(get(t, client, srv.URL+"/"))
	if m == nil {
		t.Fatal("the page has no CSRF token after sign-in")
	}
	post(t, client, srv.URL+"/items", m[1], `{"name":"Milk"}`)

	if page := get(t, client, srv.URL+"/"); !strings.Contains(page, "Milk") {
		t.Errorf("the page does not list Milk:\n%s", page)
	}
}

func get(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

// post sends a request the way Datastar sends an action: with the
// Datastar-Request header and the signals as a JSON body.
func post(t *testing.T, client *http.Client, url, csrfToken, signals string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(signals))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Content-Type", "application/json")
	if csrfToken != "" {
		req.Header.Set("X-CSRF-Token", csrfToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s: %s", url, resp.Status)
	}
}
```

The test builds the server as `cmd/server/main.go` does, with the in-memory broker and session manager. It names the same 4 type arguments as `cmd/server/main.go`, because `datapages gen` reads every `NewServer` call of the module, test files included, and fails when 2 calls for 1 app name a different session type or metrics mode. `datapages.EnablePrometheus` requires `WithPrometheus`, and a server behind `httptest` opens no metrics port.

An action that takes signals or `sse` answers 406 Not Acceptable without the header `Datastar-Request: true`. The cookie jar keeps the session cookie between requests. After the sign-in, each action needs the CSRF token, which the test reads from the script of the page and sends in `X-CSRF-Token`. Without it the action answers 403 Forbidden.

The refused guest request still answers 200, because `RecoverError` handles the `ErrForbidden` of a Datastar request and writes the alert. The test therefore checks the page, not the status.

Run the tests:

```sh
go test ./...
```

`go test` reports `ok` for the package `example.com/shoplist/app`.

## Next steps

- [Events](../events.md): subjects that address 1 user or 1 signal value, delivery and stream hooks.
- [Per-tab state](../state.md): the lifecycle of state, events for 1 tab and the instance limit.
- [Sessions and auth](../sessions.md): expiry, session managers and authorization.
- [Testing](../testing.md): streams, per-tab state and signed-in clients in tests.
- [Deployment](../deployment.md): building the binary, NATS and running several processes.
