// Drives the generated per-tab state of ./app.

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
		datapagesgen.WithStateConfig(datapagesgen.StateConfig{HMACKey: key[:]}),
	))
	t.Cleanup(srv.Close)
	return srv
}

// tab is one browser tab of one page:
// the instance id its page load minted and the stream that owns it.
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
		ctx, http.MethodGet, srv.URL+page+"_$/", nil,
	)
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
		context.Background(), http.MethodPost, tb.srv.URL+path, r,
	)
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

// TestGenericStateIsPerTab covers two tabs of the same page. The generic base
// gives both the same handlers, and each tab must still hold its own value.
func TestGenericStateIsPerTab(t *testing.T) {
	srv := newServer(t)

	a := openTab(t, srv, "/count/")
	b := openTab(t, srv, "/count/")

	if a.id == b.id {
		t.Fatal("two page loads share one instance id")
	}

	if status := a.act(t, "/count/bump/", ""); status != http.StatusOK {
		t.Fatalf("bumping the first tab: status %d", status)
	}
	if status := a.act(t, "/count/bump/", ""); status != http.StatusOK {
		t.Fatalf("bumping the first tab again: status %d", status)
	}
	if status := b.act(t, "/count/bump/", ""); status != http.StatusOK {
		t.Fatalf("bumping the second tab: status %d", status)
	}

	if !a.saw("{N:2}") {
		t.Error("the first tab did not reach 2")
	}
	if !b.saw("{N:1}") {
		t.Error("the second tab did not reach 1")
	}
	if !b.never("{N:2}") {
		t.Error("the second tab saw the first tab's count")
	}
}

// TestGenericStateIsPerType covers the same generic base instantiated on two
// state types. Each page's handlers must be given its own type's value.
func TestGenericStateIsPerType(t *testing.T) {
	srv := newServer(t)

	count := openTab(t, srv, "/count/")
	label := openTab(t, srv, "/label/")

	if status := count.act(t, "/count/bump/", ""); status != http.StatusOK {
		t.Fatalf("bumping: status %d", status)
	}
	if status := label.act(t, "/label/set/", `{"text":"hello"}`); status != http.StatusOK {
		t.Fatalf("setting the label: status %d", status)
	}

	if !count.saw("{N:1}") {
		t.Error("the counting page did not receive its own state")
	}
	if !label.saw("{Text:hello}") {
		t.Error("the labelling page did not receive its own state")
	}
	if !count.never("hello") {
		t.Error("a page received the state of a page of another type")
	}
	if !label.never("{N:") {
		t.Error("a page received the state of a page of another type")
	}
}

// TestStateFromEmbedOnly covers a page that declares no handler of its own.
//
// Its stream, its event handler and its state type come from the embedded generic page.
// The page still has to get a state value of its own,
// and an event addressed to its tab still has to arrive.
func TestStateFromEmbedOnly(t *testing.T) {
	srv := newServer(t)

	embed := openTab(t, srv, "/embed-only/")
	label := openTab(t, srv, "/label/")

	// The action belongs to another page bound to the same state type,
	// so it is the embedding page's own stream that must show its value.
	if s := label.act(t, "/label/set/", `{"text":"from label"}`); s != http.StatusOK {
		t.Fatalf("setting the label: status %d", s)
	}
	if !label.saw("{Text:from label}") {
		t.Fatal("the acting tab received nothing")
	}
	if !embed.never("from label") {
		t.Error("a tab of another page received the state")
	}

	// The embedding page opened a stream of its own, which is what allocates its state.
	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading /: %v", err)
	}
	if strings.Contains(string(b), "allocations=0") {
		t.Errorf("the embed-only page opened no state:\n%s", b)
	}
}

// TestActionOfAnotherPage covers an action of one page called with the
// instance id of a tab bound to another.
// The state types differ and there is nothing to act on.
func TestActionOfAnotherPage(t *testing.T) {
	srv := newServer(t)

	count := openTab(t, srv, "/count/")

	if status := count.act(t, "/label/set/", `{"text":"x"}`); status == http.StatusOK {
		t.Error("an action bound to another state type was served")
	}
}

// TestStateIsAllocatedPerTab covers how many state values exist.
// Two tabs are two values, which is the whole point of per-tab state.
func TestStateIsAllocatedPerTab(t *testing.T) {
	srv := newServer(t)

	_ = openTab(t, srv, "/count/")
	_ = openTab(t, srv, "/count/")
	_ = openTab(t, srv, "/label/")

	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading /: %v", err)
	}
	if !strings.Contains(string(b), "allocations=3") {
		t.Errorf("three tabs did not open three state values:\n%s", b)
	}
}
