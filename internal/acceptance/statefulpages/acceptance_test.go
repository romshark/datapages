// Drives the generated server of ./app over HTTP.

package acceptance_test

import (
	"context"
	"crypto/sha256"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/statefulpages/app"
	"github.com/romshark/datapages/internal/acceptance/statefulpages/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

const (
	pathStream           = "/_$/"
	pathAction           = "/update/"
	pathFailOpen         = "/failopen/"
	pathFailOpenStream   = "/failopen/_$/"
	pathPanicClose       = "/panicclose/"
	pathPanicCloseStream = "/panicclose/_$/"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	key := sha256.Sum256([]byte("acceptance"))
	return client.New(t, mustNewServer(t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithStateConfig(datapages.StateConfig{HMACKey: key[:]})))
}

// update sends the action that writes a tab's state.
func update(t *testing.T, tab *client.Tab, filter string) client.Response {
	t.Helper()
	return tab.Act(t, http.MethodPost, pathAction, `{"filter":"`+filter+`"}`)
}

// TestPerTabState covers what per-tab state promises: two tabs of one page
// hold separate values, and a tab-scoped event reaches the tab that caused it.
func TestPerTabState(t *testing.T) {
	c := newClient(t)

	a := c.OpenTab(t, "/", pathStream)
	b := c.OpenTab(t, "/", pathStream)

	require.NotEqual(t, a.InstanceID(), b.InstanceID(),
		"two page loads share one instance id")

	require.Equal(t, http.StatusOK, update(t, a, "from-a").Status)

	require.True(t, a.Saw("deliveries:1 filter:from-a"),
		"the tab that acted received no patch for its own state")
	require.True(t, b.Never("deliveries:"),
		"the other tab received a patch addressed at the first")

	require.Equal(t, http.StatusOK, update(t, b, "from-b").Status)
	require.True(t, b.Saw("deliveries:1 filter:from-b"),
		"the second tab does not hold its own state")
	require.True(t, a.Never("filter:from-b"), "one tab sees the state of another")
}

// TestActionWithoutInstance covers an action from a tab the server knows nothing about.
// The client learns to reconnect rather than to give up.
func TestActionWithoutInstance(t *testing.T) {
	c := newClient(t)

	// A page load mints an id. No stream follows,
	// which leaves the server without state to act on.
	page := c.Get(t, "/")
	id := page.Instance()
	require.NotEmpty(t, id, "GET / mints no instance id")

	resp := postUpdate(t, c, id, "x")
	require.Equal(t, http.StatusConflict, resp.Status)
	require.Equal(t, "reconnect", resp.Retry())
}

// TestStateResetsOnReconnect covers a tab whose stream drops.
// An instance lives exactly as long as its stream, which leaves the same id
// reconnecting onto a zeroed state rather than onto what it left behind.
//
// The counter is what shows it: a state carried over would answer with the
// second delivery, and a fresh one answers with the first.
func TestStateResetsOnReconnect(t *testing.T) {
	c := newClient(t)

	tab := c.OpenTab(t, "/", pathStream)
	require.Equal(t, http.StatusOK, update(t, tab, "before").Status)
	require.True(t, tab.Saw("deliveries:1 filter:before"),
		"no patch before the reconnect")

	tab.Close()
	tab.Reopen(t)

	require.Equal(t, http.StatusOK, update(t, tab, "after").Status)
	require.True(t, tab.Saw("deliveries:1 filter:after"),
		"the reconnected tab kept the state of the stream that dropped")
}

// TestSlowRequestDoesNotStallTab covers an action whose body arrives slowly:
// a second action on the same tab is served while the first is still being read,
// and the tab keeps receiving patches.
func TestSlowRequestDoesNotStallTab(t *testing.T) {
	c := newClient(t)
	tab := c.OpenTab(t, "/", pathStream)

	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pr.Close() })

	req, err := http.NewRequestWithContext(
		context.Background(), http.MethodPost, c.URL()+pathAction, pr,
	)
	require.NoError(t, err, "building slow request")
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Datapages-Instance", tab.InstanceID())
	req.Header.Set("Content-Type", "application/json")

	slow := make(chan *http.Response, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			slow <- nil
			return
		}
		_ = resp.Body.Close()
		slow <- resp
	}()

	// Enough of the body for the handler to start reading it,
	// not enough to finish the JSON value it waits for.
	_, err = pw.Write([]byte(`{"filter":"slo`))
	require.NoError(t, err, "writing partial body")
	time.Sleep(100 * time.Millisecond)

	// A second action from the same tab, while the first still trickles.
	require.Equal(t, http.StatusOK, update(t, tab, "fast").Status,
		"action during a slow request")
	require.True(t, tab.Saw("filter:fast"),
		"the tab received no patch while a slow request was in flight")

	_, err = pw.Write([]byte(`w"}`))
	require.NoError(t, err, "writing the rest of the body")
	require.NoError(t, pw.Close())

	select {
	case resp := <-slow:
		require.NotNil(t, resp, "the slow action failed")
		require.Equal(t, http.StatusOK, resp.StatusCode, "slow action")
	case <-time.After(5 * time.Second):
		t.Fatal("the slow action never finished")
	}
	require.True(t, tab.Saw("filter:slow"), "the slow action left no patch behind")
}

// TestShortHMACKey covers a key too short for the hash it is used with.
// A key is refused where it is given,
// rather than weakening every id the server goes on to sign with it.
func TestShortHMACKey(t *testing.T) {
	_, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](&app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithStateConfig(datapages.StateConfig{
			HMACKey: []byte("too short to sign with"),
		}))
	require.Error(t, err, "a 22 byte HMACKey was accepted")
	require.Contains(t, err.Error(), "HMACKey",
		"the refusal does not name what is wrong")
}

// TestForgedInstanceID covers an id the server never signed.
func TestForgedInstanceID(t *testing.T) {
	c := newClient(t)

	require.Equal(t, http.StatusConflict, postUpdate(t, c, "AAAA~BBBB", "x").Status)
}

// TestInstanceCap covers MaxConcurrentInstances: a stream opened past the cap
// is refused with a status and a Retry-After the client can act on.
//
// The count is process-wide, so the test opens streams until one is refused
// rather than predicting which one that is.
func TestInstanceCap(t *testing.T) {
	const capacity = 40

	key := sha256.Sum256([]byte("acceptance"))
	srv := httptest.NewServer(mustNewServer(t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithStateConfig(datapages.StateConfig{
			HMACKey:                key[:],
			MaxConcurrentInstances: capacity,
		})))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var refused *http.Response
	for i := 0; i < capacity+5 && refused == nil; i++ {
		resp, err := srv.Client().Get(srv.URL + "/")
		require.NoError(t, err, "GET /")
		_ = resp.Body.Close()
		id := resp.Header.Get("Datapages-Instance")
		require.NotEmpty(t, id, "GET / mints no instance id")

		req, err := http.NewRequestWithContext(
			ctx, http.MethodGet, srv.URL+pathStream, nil,
		)
		require.NoError(t, err, "building stream request")
		req.Header.Set("Datastar-Request", "true")
		req.Header.Set("Datapages-Instance", id)

		streamResp, err := srv.Client().Do(req)
		require.NoError(t, err, "opening stream %d", i)
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

	require.NotNil(t, refused, "streams past the cap were served instead of refused")
	defer func() { _ = refused.Body.Close() }()
	require.NotEmpty(t, refused.Header.Get("Retry-After"),
		"a refused stream does not say when to come back")
}

// TestInstanceCapDisabled covers a negative MaxConcurrentInstances,
// which removes the cap.
//
// The count of live instances is process-wide and this server ignores it.
// The check the cap performs compares that count against the configured one,
// which a negative value inverts: read literally, a server that may hold fewer
// than zero instances refuses the first stream and every stream after it.
// Opening more streams than the smallest cap would allow is what tells the two
// apart, since a cap of any size would have refused one of them by now.
func TestInstanceCapDisabled(t *testing.T) {
	const streams = 8

	key := sha256.Sum256([]byte("acceptance"))
	srv := httptest.NewServer(mustNewServer(t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithStateConfig(datapages.StateConfig{
			HMACKey:                key[:],
			MaxConcurrentInstances: -1,
		})))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	for i := range streams {
		resp, err := srv.Client().Get(srv.URL + "/")
		require.NoError(t, err, "GET /")
		_ = resp.Body.Close()
		id := resp.Header.Get("Datapages-Instance")
		require.NotEmpty(t, id, "GET / mints no instance id")

		req, err := http.NewRequestWithContext(
			ctx, http.MethodGet, srv.URL+pathStream, nil,
		)
		require.NoError(t, err, "building stream request")
		req.Header.Set("Datastar-Request", "true")
		req.Header.Set("Datapages-Instance", id)

		streamResp, err := srv.Client().Do(req)
		require.NoError(t, err, "opening stream %d", i)
		t.Cleanup(func() { _ = streamResp.Body.Close() })
		require.Equal(t, http.StatusOK, streamResp.StatusCode,
			"stream %d of a server configured to hold as many as it is asked for", i)
	}
}

// TestFailedOpenReleasesInstance covers a stream whose open hook fails after
// its instance was already reserved.
//
// The close hook that gives an instance back is wired up only once the open
// hook has succeeded, which leaves an open that fails to release what it took
// by itself. An instance it kept instead would be held for the life of the
// process, and enough of those leave no tab of any stateful page able to open
// a stream again.
// The instance count is process-wide, which leaves the capacity of a single
// server no way to observe one instance. What a released instance does show is this:
// the id that named it stops being answered. The count drops in the same place the
// id is dropped. An id the server still answers is an instance it still holds.
func TestFailedOpenReleasesInstance(t *testing.T) {
	key := sha256.Sum256([]byte("acceptance"))
	c := client.New(t, mustNewServer(t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithStateConfig(datapages.StateConfig{
			HMACKey: key[:],
		})))

	page := c.Get(t, pathFailOpen)
	require.Equal(t, http.StatusOK, page.Status, "GET %s", pathFailOpen)
	id := page.Instance()
	require.NotEmpty(t, id, "GET %s mints no instance id", pathFailOpen)

	// The instance is reserved before the open hook runs.
	// The stream is answered 200 regardless:
	// the SSE generator commits the status line first,
	// which leaves the hook nowhere to report the failure.
	req := c.Request(t, http.MethodGet, pathFailOpenStream, "")
	req.Header.Set("Datapages-Instance", id)
	require.Equal(t, http.StatusOK, c.Do(t, req).Status,
		"the stream of a page whose open fails")

	// PageIndex holds the same state type. Its action is what asks this
	// server whether the instance of the failed open is still there.
	require.True(t, client.WaitFor(func() bool {
		return postUpdate(t, c, id, "x").Status == http.StatusConflict
	}, time.Second), "the instance of a failed open is never given back: "+
		"the close hook that gives one back is wired up only for an open that "+
		"succeeded, and nothing else releases it")

	// Nothing else was disturbed:
	// a page whose open succeeds still opens and still holds state.
	tab := c.OpenTab(t, "/", pathStream)
	require.Equal(t, http.StatusOK, update(t, tab, "after-failure").Status)
	require.True(t, tab.Saw("deliveries:1 filter:after-failure"),
		"a tab opened after a failed open holds no state")
}

// TestPanicInStreamCloseIsContained covers a StreamClose that panics.
//
// The hook runs on the watchdog goroutine, which is not one net/http recovers,
// and it holds the slot mutex while it runs. An unrecovered panic there ends
// the process for every user of the server. A recovery that skips the unlock
// and the release wedges the tab instead and keeps its instance forever.
func TestPanicInStreamCloseIsContained(t *testing.T) {
	key := sha256.Sum256([]byte("acceptance"))
	c := client.New(t, mustNewServer(t, &app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithStateConfig(datapages.StateConfig{
			HMACKey: key[:],
		})))

	tab := c.OpenTab(t, pathPanicClose, pathPanicCloseStream)
	id := tab.InstanceID()
	tab.Close()

	require.Equal(t, http.StatusOK, c.Get(t, pathPanicClose).Status,
		"the server stopped serving after a panicking close hook")

	// PageIndex holds the same state type, which is what lets this ask whether
	// the instance the panicking hook held was given back. A mutex the hook
	// never unlocked shows up as an action that never returns. The deadline on
	// the probe turns that into a failure with a message. It cannot keep the
	// case quick, since closing the test server waits on the wedged handler.
	probe := func() int {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.URL()+pathAction, strings.NewReader(`{"filter":"x"}`))
		require.NoError(t, err, "building the probe")
		req.Header.Set("Datastar-Request", "true")
		req.Header.Set("Datapages-Instance", id)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err,
			"the action never returned, which is the slot mutex still held "+
				"by the hook that panicked under it")
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	require.True(t, client.WaitFor(func() bool {
		return probe() == http.StatusConflict
	}, time.Second), "the instance of a panicking close hook is never given "+
		"back: the release runs after the hook, and only a deferred one "+
		"runs at all")
}

// postUpdate sends the action as the tab named by instanceID,
// which is how a test reaches the server without holding a tab.
func postUpdate(
	t *testing.T, c *client.Client, instanceID, filter string,
) client.Response {
	t.Helper()
	req := c.Request(t, http.MethodPost, pathAction, `{"filter":"`+filter+`"}`)
	req.Header.Set("Datapages-Instance", instanceID)
	return c.Do(t, req)
}
