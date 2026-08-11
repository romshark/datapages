// Drives the generated server of ./app over HTTP.

package acceptance_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dpacceptance/app"
	"dpacceptance/datapagesgen"

	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

const (
	pathStream = "/_$/"
	pathAction = "/update/"
)

// TestPerTabState covers what per-tab state promises: two tabs of one page
// hold separate values, and a tab-scoped event reaches the tab that caused it.
func TestPerTabState(t *testing.T) {
	srv := newServer(t)

	a := openTab(t, srv)
	b := openTab(t, srv)

	if a.instanceID == b.instanceID {
		t.Fatal("two page loads share one instance id")
	}

	a.update(t, "from-a")

	if got := a.await(t, "deliveries:1 filter:from-a"); !got {
		t.Error("the tab that acted received no patch for its own state")
	}
	if b.await(t, "deliveries:") {
		t.Error("the other tab received a patch addressed at the first")
	}

	b.update(t, "from-b")
	if got := b.await(t, "deliveries:1 filter:from-b"); !got {
		t.Error("the second tab does not hold its own state")
	}
	if a.await(t, "filter:from-b") {
		t.Error("one tab sees the state of another")
	}
}

// TestActionWithoutInstance covers an action from a tab the server knows
// nothing about. The client learns to reconnect rather than to give up.
func TestActionWithoutInstance(t *testing.T) {
	srv := newServer(t)

	// A page load mints an id. No stream follows, which leaves the server
	// without state to act on.
	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	_ = resp.Body.Close()
	id := resp.Header.Get("Datapages-Instance")
	if id == "" {
		t.Fatal("GET / mints no instance id")
	}

	code, retry := postUpdate(t, srv, id, "x")
	if code != http.StatusConflict {
		t.Errorf("status = %d, want %d", code, http.StatusConflict)
	}
	if retry != "reconnect" {
		t.Errorf("Datapages-Retry = %q, want %q", retry, "reconnect")
	}
}

// TestStateSurvivesReconnect covers a tab whose stream drops. Within the
// grace period the same id finds the same state.
func TestStateSurvivesReconnect(t *testing.T) {
	srv := newServer(t)

	tab := openTab(t, srv)
	tab.update(t, "before")
	if !tab.await(t, "deliveries:1 filter:before") {
		t.Fatal("no patch before the reconnect")
	}

	tab.closeStream()
	tab.reopen(t, srv)

	tab.update(t, "after")
	if !tab.await(t, "deliveries:2 filter:after") {
		t.Error("the reconnected tab starts from a fresh state")
	}
}

// TestForgedInstanceID covers an id the server never signed.
func TestForgedInstanceID(t *testing.T) {
	srv := newServer(t)

	code, _ := postUpdate(t, srv, "AAAA~BBBB", "x")
	if code != http.StatusConflict {
		t.Errorf("status = %d, want %d", code, http.StatusConflict)
	}
}

// TestInstanceCap covers MaxConcurrentInstances.
//
// A page load plus a stream connect creates an instance, and anyone who can
// reach the server can ask for one. Without a cap that is unbounded memory
// handed out to unauthenticated callers. With one, the connection past it has
// to be refused in a way the client can act on rather than served an instance
// that does not exist.
//
// The registry the cap counts is process-wide rather than per server. The
// instances other tests in this binary still hold count against it. The test
// opens streams until one is refused instead of predicting which one that
// is.
func TestInstanceCap(t *testing.T) {
	const capacity = 40

	key := sha256.Sum256([]byte("acceptance"))
	srv := httptest.NewServer(datapagesgen.NewServer(&app.App{}, inmem.New(8),
		datapagesgen.WithStateConfig(datapagesgen.StateConfig{
			HMACKey:                key[:],
			MaxConcurrentInstances: capacity,
		})))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var refused *http.Response
	for i := 0; i < capacity+5 && refused == nil; i++ {
		resp, err := srv.Client().Get(srv.URL + "/")
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		_ = resp.Body.Close()
		id := resp.Header.Get("Datapages-Instance")
		if id == "" {
			t.Fatal("GET / mints no instance id")
		}

		req, err := http.NewRequestWithContext(
			ctx, http.MethodGet, srv.URL+pathStream, nil)
		if err != nil {
			t.Fatalf("building stream request: %v", err)
		}
		req.Header.Set("Datastar-Request", "true")
		req.Header.Set("Datapages-Instance", id)

		streamResp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("opening stream %d: %v", i, err)
		}
		switch streamResp.StatusCode {
		case http.StatusOK:
			t.Cleanup(func() { _ = streamResp.Body.Close() })
		case http.StatusServiceUnavailable:
			refused = streamResp
		default:
			_ = streamResp.Body.Close()
			t.Fatalf("opening stream %d: status %d", i, streamResp.StatusCode)
		}
	}

	if refused == nil {
		t.Fatal("streams past the cap were served instead of refused")
	}
	defer func() { _ = refused.Body.Close() }()
	if refused.Header.Get("Retry-After") == "" {
		t.Error("a refused stream does not say when to come back")
	}
}

// --- harness ---------------------------------------------------------------

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	key := sha256.Sum256([]byte("acceptance"))
	s := datapagesgen.NewServer(&app.App{}, inmem.New(8),
		datapagesgen.WithStateConfig(datapagesgen.StateConfig{
			HMACKey: key[:],
		}))
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return srv
}

// tab is one browser tab: an instance id plus the SSE stream that owns it.
type tab struct {
	srv        *httptest.Server
	instanceID string
	cancel     context.CancelFunc

	mu    sync.Mutex
	lines []string
}

func openTab(t *testing.T, srv *httptest.Server) *tab {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	_ = resp.Body.Close()
	id := resp.Header.Get("Datapages-Instance")
	if id == "" {
		t.Fatal("GET / mints no instance id")
	}
	tb := &tab{srv: srv, instanceID: id}
	tb.reopen(t, srv)
	return tb
}

// reopen connects the SSE stream the way the browser does, with the id the
// page load handed out.
func (tb *tab) reopen(t *testing.T, srv *httptest.Server) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	tb.cancel = cancel
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, srv.URL+pathStream, nil)
	if err != nil {
		t.Fatalf("building stream request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Datapages-Instance", tb.instanceID)

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("opening stream: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("opening stream: status %d", resp.StatusCode)
	}
	go func() {
		defer func() { _ = resp.Body.Close() }()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			tb.mu.Lock()
			tb.lines = append(tb.lines, sc.Text())
			tb.mu.Unlock()
		}
	}()
	// StreamOpen registers the instance. An action that arrives first is
	// answered with 409 by design.
	waitFor(func() bool { return true }, 100*time.Millisecond)
}

func (tb *tab) closeStream() {
	tb.cancel()
	time.Sleep(100 * time.Millisecond)
}

func (tb *tab) update(t *testing.T, filter string) {
	t.Helper()
	code, _ := postUpdate(t, tb.srv, tb.instanceID, filter)
	if code != http.StatusOK {
		t.Fatalf("POST %s: status %d", pathAction, code)
	}
}

// await reports whether sub shows up on the stream within a short window.
func (tb *tab) await(t *testing.T, sub string) bool {
	t.Helper()
	return waitFor(func() bool {
		tb.mu.Lock()
		defer tb.mu.Unlock()
		for _, l := range tb.lines {
			if strings.Contains(l, sub) {
				return true
			}
		}
		return false
	}, time.Second)
}

func waitFor(cond func() bool, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if cond() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func postUpdate(
	t *testing.T, srv *httptest.Server, instanceID, filter string,
) (status int, retry string) {
	t.Helper()
	body := strings.NewReader(`{"filter":"` + filter + `"}`)
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+pathAction, body)
	if err != nil {
		t.Fatalf("building action request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Datapages-Instance", instanceID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", pathAction, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, resp.Header.Get("Datapages-Retry")
}
