// Drives the generated SSE machinery of ./app.

package acceptance_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
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
	srv := httptest.NewServer(datapagesgen.NewServer(&app.App{}, inmem.New(8)))
	t.Cleanup(srv.Close)
	return srv
}

// stream is one open SSE connection, the way a browser tab holds one.
type stream struct {
	cancel context.CancelFunc

	mu    sync.Mutex
	lines []string
}

// open connects to a page's stream. signals are the ones the client sends at
// connect time, which is where subject scoping gets its values.
func open(t *testing.T, srv *httptest.Server, path string, signals any) *stream {
	t.Helper()

	target := srv.URL + path + "_$/"
	if signals != nil {
		b, err := json.Marshal(signals)
		if err != nil {
			t.Fatalf("encoding signals: %v", err)
		}
		target += "?datastar=" + url.QueryEscape(string(b))
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		t.Fatalf("building stream request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("opening stream %s: %v", target, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		t.Fatalf("opening stream %s: status %d", target, resp.StatusCode)
	}

	s := &stream{cancel: cancel}
	go func() {
		defer func() { _ = resp.Body.Close() }()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			s.mu.Lock()
			s.lines = append(s.lines, sc.Text())
			s.mu.Unlock()
		}
	}()
	// The subscription is registered by the handler, not by the response
	// header. A dispatch sent immediately can outrun it.
	time.Sleep(settle)
	return s
}

func (s *stream) close() {
	s.cancel()
	time.Sleep(settle)
}

// saw reports whether sub showed up on the stream within a short window.
func (s *stream) saw(sub string) bool {
	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		for _, l := range s.lines {
			if strings.Contains(l, sub) {
				s.mu.Unlock()
				return true
			}
		}
		s.mu.Unlock()
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// never reports whether sub stayed off the stream for a short window. It waits
// the window out. Only then does a negative mean anything.
func (s *stream) never(sub string) bool {
	time.Sleep(settle)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range s.lines {
		if strings.Contains(l, sub) {
			return false
		}
	}
	return true
}

func post(t *testing.T, srv *httptest.Server, path, body string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building %s: %v", path, err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s: status %d", path, resp.StatusCode)
	}
}

func logOf(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + "/log/")
	if err != nil {
		t.Fatalf("GET /log/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading /log/: %v", err)
	}
	body := string(b)
	const openTag = `<pre id="echo">`
	i := strings.Index(body, openTag)
	if i < 0 {
		t.Fatalf("no echo element in /log/:\n%s", body)
	}
	rest := body[i+len(openTag):]
	j := strings.Index(rest, "</pre>")
	if j < 0 {
		t.Fatalf("unterminated echo element in /log/:\n%s", body)
	}
	return rest[:j]
}

// TestPublicEventReachesEveryStream covers an event with no subject fields.
// Every stream of a page that handles it receives it.
func TestPublicEventReachesEveryStream(t *testing.T) {
	srv := newServer(t)

	a := open(t, srv, "/", nil)
	b := open(t, srv, "/", nil)

	post(t, srv, "/tick/", `{"n":7}`)

	if !a.saw(`<div id="out">tick 7</div>`) {
		t.Error("the first stream received no patch")
	}
	if !b.saw(`<div id="out">tick 7</div>`) {
		t.Error("the second stream received no patch")
	}
}

// TestStreamHooks covers the two hooks that bracket a stream's life. Both must
// run and both must see the same stream id. An application uses that id to
// pair up what it allocated with what it releases.
func TestStreamHooks(t *testing.T) {
	srv := newServer(t)

	s := open(t, srv, "/", nil)
	opened := logOf(t, srv)
	if !strings.HasPrefix(opened, "open(") {
		t.Fatalf("StreamOpen did not run: %q", opened)
	}
	id := strings.TrimSuffix(strings.TrimPrefix(opened, "open("), ")")

	s.close()

	if got, want := logOf(t, srv), "open("+id+") close("+id+")"; got != want {
		t.Errorf(" got: %s\nwant: %s", got, want)
	}
}

// TestStreamIDReachesEventHandler covers the stream id an event handler may
// ask for. It must name the stream the handler is writing on, not some other.
func TestStreamIDReachesEventHandler(t *testing.T) {
	srv := newServer(t)

	_ = open(t, srv, "/", nil)
	opened := logOf(t, srv)
	id := strings.TrimSuffix(strings.TrimPrefix(opened, "open("), ")")

	post(t, srv, "/tick/", `{"n":3}`)
	time.Sleep(settle)

	if got, want := logOf(t, srv), "open("+id+") tick("+id+",3)"; got != want {
		t.Errorf(" got: %s\nwant: %s", got, want)
	}
}

// TestEmbeddedHandler covers a page that declares no event handler of its own
// and embeds one. The page receives the event through the embedded type.
func TestEmbeddedHandler(t *testing.T) {
	srv := newServer(t)

	other := open(t, srv, "/other/", nil)
	post(t, srv, "/tick/", `{"n":1}`)

	if !other.saw(`<div id="shared">shared 1</div>`) {
		t.Error("the embedding page received nothing from the embedded handler")
	}
}

// TestSubjectScoping covers an event whose subject carries a value the stream
// chose when it connected. Two streams of one page, two values, one dispatch:
// only the addressed stream may see it.
func TestSubjectScoping(t *testing.T) {
	srv := newServer(t)

	red := open(t, srv, "/room/", map[string]string{"room": "red"})
	blue := open(t, srv, "/room/", map[string]string{"room": "blue"})

	post(t, srv, "/room/say/", `{"room":"red","text":"hello red"}`)

	if !red.saw(`<div id="said">hello red</div>`) {
		t.Error("the addressed stream received nothing")
	}
	if !blue.never("hello red") {
		t.Error("a stream received an event addressed at another")
	}
}

// TestSubjectFanout covers the plural form of a subject field. One dispatch
// carrying two values is published to both subjects. Both streams see it and a
// third one does not.
func TestSubjectFanout(t *testing.T) {
	srv := newServer(t)

	red := open(t, srv, "/room/", map[string]string{"room": "red"})
	blue := open(t, srv, "/room/", map[string]string{"room": "blue"})
	green := open(t, srv, "/room/", map[string]string{"room": "green"})

	post(t, srv, "/room/broadcast/",
		`{"rooms":["red","blue"],"text":"to both"}`)

	if !red.saw(`<div id="broadcast">to both</div>`) {
		t.Error("the first addressed stream received nothing")
	}
	if !blue.saw(`<div id="broadcast">to both</div>`) {
		t.Error("the second addressed stream received nothing")
	}
	if !green.never("to both") {
		t.Error("a stream outside the dispatch received the event")
	}
}

// TestStreamOpenRefuses covers a StreamOpen that fails.
//
// The hook is where an application acquires what the stream needs and where it
// decides the stream may not run at all. A failure there must end the request
// rather than leave a connection open that no handler is behind, and it must
// not run StreamClose for a stream that never opened.
func TestStreamOpenRefuses(t *testing.T) {
	srv := newServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, srv.URL+"/_$/?refuse=1", nil)
	if err != nil {
		t.Fatalf("building stream request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /_$/?refuse=1: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Whatever the status, the response has to end rather than hang: the
	// client is waiting on a stream that will never carry anything.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(io.Discard, resp.Body)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a refused stream was left open")
	}

	if got := logOf(t, srv); strings.Contains(got, "open(") {
		t.Errorf("the refused stream was recorded as opened: %s", got)
	}
	if got := logOf(t, srv); strings.Contains(got, "close(") {
		t.Errorf("StreamClose ran for a stream that never opened: %s", got)
	}

	// The server keeps serving streams that are not refused.
	s := open(t, srv, "/", nil)
	post(t, srv, "/tick/", `{"n":9}`)
	if !s.saw(`<div id="out">tick 9</div>`) {
		t.Error("a later stream was not served")
	}
}

// TestStreamRequiresDatastar covers a stream route reached by a plain client.
func TestStreamRequiresDatastar(t *testing.T) {
	srv := newServer(t)

	resp, err := srv.Client().Get(srv.URL + "/_$/")
	if err != nil {
		t.Fatalf("GET /_$/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotAcceptable {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotAcceptable)
	}
}

// TestMissingSubjectSignal covers a stream that connects without the signal
// its subscription is scoped by. There is nothing to subscribe to, and the
// client is told so rather than served a stream that stays silent.
func TestMissingSubjectSignal(t *testing.T) {
	srv := newServer(t)

	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodGet, srv.URL+"/room/_$/", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /room/_$/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

// TestBrokerStreamInitialization covers the hand-off to a message broker that
// needs its streams created before anything is published to them.
//
// A broker that implements msgbroker.StreamInitializer is given every subject
// the app can publish, once, at startup. A NATS JetStream deployment cannot
// work without it, and nothing on the datapages side reports a subject that
// was left out: the publish simply goes nowhere.
func TestBrokerStreamInitialization(t *testing.T) {
	broker := &initializingBroker{MessageBroker: inmem.New(8)}
	srv := httptest.NewServer(datapagesgen.NewServer(&app.App{}, broker))
	t.Cleanup(srv.Close)

	if broker.calls != 1 {
		t.Fatalf("InitStreams ran %d times, want once", broker.calls)
	}
	want := datapagesgen.MessageBrokerStreamSubjects()
	if len(want) == 0 {
		t.Fatal("an app with events exports no stream subjects")
	}
	if !slices.Equal(broker.subjects, want) {
		t.Errorf("InitStreams received %v, want %v", broker.subjects, want)
	}

	// The subjects have to cover what the app publishes, or the events that
	// use them have nowhere to go.
	for _, base := range []string{"tick", "room.said", "room.broadcast"} {
		var found bool
		for _, s := range broker.subjects {
			if strings.HasPrefix(s, base) {
				found = true
			}
		}
		if !found {
			t.Errorf("no stream subject covers %q: %v", base, broker.subjects)
		}
	}
}

// TestBrokerInitFailureIsFatal covers a broker that cannot create its streams.
// Starting anyway would mean a server that accepts requests and drops every
// event it publishes.
func TestBrokerInitFailureIsFatal(t *testing.T) {
	broker := &initializingBroker{
		MessageBroker: inmem.New(8),
		err:           errors.New("the stream could not be created"),
	}

	defer func() {
		if recover() == nil {
			t.Error("a broker that cannot create its streams was accepted")
		}
	}()
	_ = datapagesgen.NewServer(&app.App{}, broker)
}

// initializingBroker is a broker that needs its streams set up. The generated
// server calls InitStreams on such a broker.
type initializingBroker struct {
	*inmem.MessageBroker
	subjects []string
	calls    int
	err      error
}

func (b *initializingBroker) InitStreams(subjects []string) error {
	b.calls++
	b.subjects = append([]string(nil), subjects...)
	return b.err
}

// TestPageWithoutStream covers a page that handles no events. It has no
// stream route at all, and asking for one is a 404 rather than a stream that
// never carries anything.
func TestPageWithoutStream(t *testing.T) {
	srv := newServer(t)

	req, err := http.NewRequestWithContext(context.Background(),
		http.MethodGet, srv.URL+"/log/_$/", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Datastar-Request", "true")

	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /log/_$/: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}
