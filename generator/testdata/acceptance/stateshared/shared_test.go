// Drives a state type shared by two pages.

package acceptance_test

import (
	"bufio"
	"context"
	"crypto/sha256"
	"io"
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

const settle = 200 * time.Millisecond

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	key := sha256.Sum256([]byte("acceptance"))
	srv := httptest.NewServer(datapagesgen.NewServer(
		&app.App{}, inmem.New(8),
		datapagesgen.WithStateConfig(datapagesgen.StateConfig{HMACKey: key[:]})))
	t.Cleanup(srv.Close)
	return srv
}

// tab is one browser tab of one page: the instance id its page load minted
// and the stream that owns it.
type tab struct {
	srv    *httptest.Server
	page   string
	id     string
	cancel context.CancelFunc

	mu    sync.Mutex
	lines []string
}

func openTab(t *testing.T, srv *httptest.Server, page string) *tab {
	t.Helper()

	resp, err := srv.Client().Get(srv.URL + page)
	if err != nil {
		t.Fatalf("GET %s: %v", page, err)
	}
	_ = resp.Body.Close()
	id := resp.Header.Get("Datapages-Instance")
	if id == "" {
		t.Fatalf("GET %s mints no instance id", page)
	}

	tb := &tab{srv: srv, page: page, id: id}

	ctx, cancel := context.WithCancel(context.Background())
	tb.cancel = cancel
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, srv.URL+page+"_$/", nil)
	if err != nil {
		t.Fatalf("building stream request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Datapages-Instance", id)
	req.Header.Set("Accept-Encoding", "identity")

	sresp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("opening stream on %s: %v", page, err)
	}
	if sresp.StatusCode != http.StatusOK {
		_ = sresp.Body.Close()
		t.Fatalf("opening stream on %s: status %d", page, sresp.StatusCode)
	}
	go func() {
		defer func() { _ = sresp.Body.Close() }()
		sc := bufio.NewScanner(sresp.Body)
		for sc.Scan() {
			tb.mu.Lock()
			tb.lines = append(tb.lines, sc.Text())
			tb.mu.Unlock()
		}
	}()
	time.Sleep(settle)
	return tb
}

func (tb *tab) act(t *testing.T, path, body string) int {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, tb.srv.URL+path, r)
	if err != nil {
		t.Fatalf("building POST %s: %v", path, err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Datapages-Instance", tb.id)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := tb.srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func (tb *tab) saw(sub string) bool {
	deadline := time.Now().Add(time.Second)
	for {
		tb.mu.Lock()
		for _, l := range tb.lines {
			if strings.Contains(l, sub) {
				tb.mu.Unlock()
				return true
			}
		}
		tb.mu.Unlock()
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (tb *tab) never(sub string) bool {
	time.Sleep(settle)
	tb.mu.Lock()
	defer tb.mu.Unlock()
	for _, l := range tb.lines {
		if strings.Contains(l, sub) {
			return false
		}
	}
	return true
}

// TestSharedStateIsPerTab covers two tabs of one page bound to a shared state
// type. Sharing the type must not share the value.
func TestSharedStateIsPerTab(t *testing.T) {
	srv := newServer(t)

	a := openTab(t, srv, "/")
	b := openTab(t, srv, "/")

	if status := a.act(t, "/bump/", ""); status != http.StatusOK {
		t.Fatalf("bumping the first tab: status %d", status)
	}
	if status := a.act(t, "/bump/", ""); status != http.StatusOK {
		t.Fatalf("bumping the first tab again: status %d", status)
	}

	if !a.saw("Counter:2") {
		t.Error("the first tab did not reach 2")
	}
	if !b.never("Counter:") {
		t.Error("the second tab received the first tab's state")
	}
}

// TestSharedStateIsPerPage covers two different pages bound to the same state
// type. Their tabs are still separate tabs.
func TestSharedStateIsPerPage(t *testing.T) {
	srv := newServer(t)

	index := openTab(t, srv, "/")
	other := openTab(t, srv, "/other/")

	if status := other.act(t, "/bump/", ""); status != http.StatusOK {
		t.Fatalf("bumping the other page: status %d", status)
	}

	if !other.saw("Counter:1") {
		t.Error("the page that acted received nothing")
	}
	if !index.never("Counter:1") {
		t.Error("a tab of another page received the state")
	}
}

// TestPageActionAndAppActionShareTheState covers the two ways into one tab's
// state: an action of the page and an action of the app.
func TestPageActionAndAppActionShareTheState(t *testing.T) {
	srv := newServer(t)

	tab := openTab(t, srv, "/")

	if status := tab.act(t, "/note/", `{"note":"hello"}`); status != http.StatusOK {
		t.Fatalf("setting the note: status %d", status)
	}
	if !tab.saw("Note:hello") {
		t.Fatal("the page action did not write the state")
	}

	if status := tab.act(t, "/bump/", ""); status != http.StatusOK {
		t.Fatalf("bumping: status %d", status)
	}
	if !tab.saw("{Counter:1 Note:hello}") {
		t.Error("the app action did not act on the same value the page action wrote")
	}
}

// TestAppActionFromAStatelessPage covers a tab of a page that uses no state
// calling an app action that needs one. There is nothing to act on. The client
// is told to reconnect instead of being served another tab's value.
func TestAppActionFromAStatelessPage(t *testing.T) {
	srv := newServer(t)

	resp, err := srv.Client().Get(srv.URL + "/plain/")
	if err != nil {
		t.Fatalf("GET /plain/: %v", err)
	}
	_ = resp.Body.Close()
	id := resp.Header.Get("Datapages-Instance")

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, srv.URL+"/bump/", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	if id != "" {
		req.Header.Set("Datapages-Instance", id)
	}

	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /bump/: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusConflict)
	}
}
