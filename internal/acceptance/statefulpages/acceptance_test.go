// Drives the generated server of ./app over HTTP.

package acceptance_test

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/statefulpages/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

const (
	pathStream         = "/_$/"
	pathAction         = "/update/"
	pathExport         = "/export/"
	pathFailOpen       = "/failopen/"
	pathFailOpenStream = "/failopen/_$/"
	pathPanicClose     = "/panicclose/"
	pathCloseState     = "/closestate/"
	pathCloseStateMark = "/closestate/mark/"
)

func serve(t *testing.T, opts ...datapages.ServerOption) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer), opts...))
}

// update sends the action that writes a tab's state.
func update(t *testing.T, tab *datapagestest.Tab, filter string) *datapagestest.Response {
	t.Helper()
	return tab.Do(t, http.MethodPost, pathAction, map[string]string{"filter": filter})
}

// status matches the patch of a tab's state, such as "deliveries:1 filter:a".
func status(text string) datapagestest.Match {
	return datapagestest.TextContains("status", text)
}

// TestPerTabState covers what per-tab state promises: 2 tabs of 1 page
// hold separate values, and a tab-scoped event reaches the tab that caused it.
func TestPerTabState(t *testing.T) {
	v := serve(t).Visitor(t)

	aTab := v.Load(t, "/")
	a := aTab.Connect(t, nil)
	bTab := v.Load(t, "/")
	b := bTab.Connect(t, nil)

	require.NotEqual(t, aTab.Instance(), bTab.Instance(),
		"2 page loads share 1 instance id")

	require.Equal(t, http.StatusOK, update(t, aTab, "from-a").Status)
	a.WaitFor(t, status("deliveries:1 filter:from-a"),
		"the tab that acted received no patch for its own state")

	// Each tab's own update is the anchor. A patch for the other tab's event would
	// render the receiving tab's own state and come before the anchor.
	require.Equal(t, http.StatusOK, update(t, bTab, "from-b").Status)
	b.Absent(t, datapagestest.Element("status"), status("deliveries:1 filter:from-b"),
		"the other tab received a patch addressed at the first")

	require.Equal(t, http.StatusOK, update(t, aTab, "again").Status)
	a.Absent(t, datapagestest.Element("status"), status("filter:again"),
		"one tab sees the state of another")
}

// TestSecondStreamUnderTheSameID covers a tab that opens a second stream before
// the server has noticed its first one is gone, which is what a Datastar
// reconnect does over a connection that dropped without closing.
//
// The second stream registers a slot of its own under the same id.
// The first one is still on its way out and releases by the slot it allocated
// rather than by the id, which is what keeps it from taking the live slot with it.
// Releasing by id would leave the tab holding a page whose every action is
// answered 409 while its stream is open.
func TestSecondStreamUnderTheSameID(t *testing.T) {
	srv := serve(t)
	tab := srv.Visitor(t).Load(t, "/")
	first := tab.Connect(t, nil)
	second := tab.Connect(t, nil)

	first.Close()

	srv.WaitFor(t, "an action of the tab to find the second stream's state", func() bool {
		return update(t, tab, "after-first-closed").Status == http.StatusOK
	})
	second.WaitFor(t, status("filter:after-first-closed"))

	// The slot is the second stream's to release, and nothing else holds one.
	second.Close()
	srv.WaitFor(t, "the state to be released with the last stream that named it",
		func() bool {
			return update(t, tab, "after-both-closed").Status == http.StatusConflict
		})
}

// TestStreamCloseReadsTheFinalState covers the value a stateful StreamClose is
// given: what the tab's last handler wrote, not the zero value its stream
// started with.
//
// The hook takes no sse and runs once the tab is gone, which leaves the app
// the only place it can report from.
func TestStreamCloseReadsTheFinalState(t *testing.T) {
	a := &app.App{}
	srv := datapagestest.New(t,
		mustNewServer(t, a, inmem.New(messaging.DefaultBrokerChanBuffer)))

	tab := srv.Visitor(t).Load(t, pathCloseState)
	st := tab.Connect(t, nil)
	resp := tab.Do(t, http.MethodPost, pathCloseStateMark,
		map[string]string{"filter": "written"})
	require.Equal(t, http.StatusOK, resp.Status,
		"writing the state the close hook has to find")
	require.Empty(t, a.Closed(), "the close hook ran while the stream was open")

	st.Close()

	srv.WaitFor(t, "StreamClose to read what the action wrote", func() bool {
		return slices.Contains(a.Closed(), "written")
	})
}

// TestActionWithoutInstance covers an action from a tab the server knows nothing about.
// The client learns to reconnect rather than to give up.
func TestActionWithoutInstance(t *testing.T) {
	// A page load mints an id. No stream follows,
	// which leaves the server without state to act on.
	tab := serve(t).Visitor(t).Load(t, "/")
	require.NotEmpty(t, tab.Instance(), "GET / mints no instance id")

	resp := update(t, tab, "x")
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
	tab := serve(t).Visitor(t).Load(t, "/")
	st := tab.Connect(t, nil)
	require.Equal(t, http.StatusOK, update(t, tab, "before").Status)
	st.WaitFor(t, status("deliveries:1 filter:before"))

	st.Close()
	st = tab.Connect(t, nil)

	require.Equal(t, http.StatusOK, update(t, tab, "after").Status)
	st.WaitFor(t, status("deliveries:1 filter:after"))
}

// TestSlowRequestDoesNotStallTab covers an action whose body arrives slowly:
// a second action on the same tab is served while the first is still being read,
// and the tab keeps receiving patches.
func TestSlowRequestDoesNotStallTab(t *testing.T) {
	v := serve(t).Visitor(t)
	tab := v.Load(t, "/")
	st := tab.Connect(t, nil)

	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pr.Close() })

	req := tab.NewRequest(t, http.MethodPost, pathAction, pr)
	req.Header.Set("Content-Type", "application/json")

	slow := make(chan *http.Response, 1)
	go func() {
		resp, err := v.Client().Do(req)
		if err != nil {
			slow <- nil
			return
		}
		_ = resp.Body.Close()
		slow <- resp
	}()

	// Enough of the body for the handler to start reading it,
	// not enough to finish the JSON value it waits for.
	_, err := pw.Write([]byte(`{"filter":"slo`))
	require.NoError(t, err, "writing partial body")
	time.Sleep(100 * time.Millisecond)

	// A second action from the same tab, while the first still trickles.
	require.Equal(t, http.StatusOK, update(t, tab, "fast").Status,
		"action during a slow request")
	st.WaitFor(t, status("filter:fast"))

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
	st.WaitFor(t, status("filter:slow"))
}

// TestSlowFileDoesNotStallTab tests an action that answers with a file the
// client doesn't read: a second action on the same tab is served while the
// server still writes the file, and the tab keeps receiving patches.
func TestSlowFileDoesNotStallTab(t *testing.T) {
	v := serve(t).Visitor(t)
	tab := v.Load(t, "/")
	st := tab.Connect(t, nil)

	export, err := v.Client().Do(tab.NewRequest(t, http.MethodPost, pathExport, nil))
	require.NoError(t, err, "export")
	// Closing the unread body ends the export.
	t.Cleanup(func() { _ = export.Body.Close() })
	require.Equal(t, http.StatusOK, export.StatusCode, "export")
	require.Equal(t, strconv.Itoa(app.ExportSize), export.Header.Get("Content-Length"))

	req := tab.NewRequest(t, http.MethodPost, pathAction,
		strings.NewReader(`{"filter":"fast"}`))
	req.Header.Set("Content-Type", "application/json")
	answered := make(chan int, 1)
	go func() {
		resp, err := v.Client().Do(req)
		if err != nil {
			answered <- 0
			return
		}
		_ = resp.Body.Close()
		answered <- resp.StatusCode
	}()
	select {
	case status := <-answered:
		require.Equal(t, http.StatusOK, status, "action during an unread export")
	case <-time.After(5 * time.Second):
		t.Fatal("the action waited for the client to read the export")
	}
	st.WaitFor(t, status("filter:fast"),
		"the tab received no patch while the export was unread")
}

// TestMalformedInstanceID tests that a wrong length or a character outside
// the base64url alphabet makes streams and actions return the reconnect
// response without allocating state.
func TestMalformedInstanceID(t *testing.T) {
	v := serve(t).Visitor(t)

	for name, id := range map[string]string{
		"empty":     "",
		"short":     "AAAA",
		"long":      strings.Repeat("A", 23),
		"separator": "AAAAAAAAAAAAAAAAAAAA~B",
		"quote":     `AAAAAAAAAAAAAAAAAAAAA"`,
		"padding":   "AAAAAAAAAAAAAAAAAAAAA=",
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, http.StatusConflict, openStream(t, v, id),
				"the stream of a malformed id was opened")
			resp := postUpdate(t, v, id, "x")
			require.Equal(t, http.StatusConflict, resp.Status,
				"the action of a malformed id was served")
			require.Equal(t, "reconnect", resp.Retry())
		})
	}
}

// TestUnmintedInstanceID tests that a well-formed id need not come from this server.
// Before its stream connects, an action receives the reconnect response.
// The stream then creates state under the id, and a later action uses that state.
func TestUnmintedInstanceID(t *testing.T) {
	v := serve(t).Visitor(t)
	const id = "AAAAAAAAAAAAAAAAAAAAAA" // 22 characters, the shape the server mints.

	resp := postUpdate(t, v, id, "x")
	require.Equal(t, http.StatusConflict, resp.Status,
		"an action was served for an id without a stream")
	require.Equal(t, "reconnect", resp.Retry())

	stream, err := v.Client().Do(streamRequest(t, v, id))
	require.NoError(t, err, "opening the stream of an unminted id")
	t.Cleanup(func() { _ = stream.Body.Close() })
	require.Equal(t, http.StatusOK, stream.StatusCode,
		"the stream of a well-formed id the server did not mint was refused")

	// State registration precedes the HTTP 200 response.
	// The action can find the state as soon as the client sees that the stream is open.
	require.Equal(t, http.StatusOK, postUpdate(t, v, id, "x").Status,
		"the stream allocated no state under the supplied id")
}

// capServer starts a server whose state budget is cap and returns a visitor of it.
func capServer(t *testing.T, cap int) *datapagestest.Visitor {
	t.Helper()
	return serve(t, datapages.WithStateConfig(datapages.StateConfig{
		MaxConcurrentInstances: cap,
	})).Visitor(t)
}

// openInstance loads the page as v and opens the stream of the id it mints.
// The stream stays open until the test ends, which is what holds the
// instance the caller is counting.
func openInstance(t *testing.T, v *datapagestest.Visitor) int {
	t.Helper()
	tab := v.Load(t, "/")
	require.NotEmpty(t, tab.Instance(), "GET / mints no instance id")
	resp, err := v.Client().Do(tab.NewRequest(t, http.MethodGet, pathStream, nil))
	require.NoError(t, err, "opening stream")
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode == http.StatusServiceUnavailable {
		require.NotEmpty(t, resp.Header.Get("Retry-After"),
			"a refused stream does not say when to come back")
	}
	return resp.StatusCode
}

// TestInstanceCap covers MaxConcurrentInstances: a stream opened past the cap
// is refused with a status and a Retry-After the client can act on.
//
// The count belongs to the server, which is what makes the refusal fall on a
// stream the test can name rather than on whichever one crosses a shared total.
func TestInstanceCap(t *testing.T) {
	const capacity = 3

	v := capServer(t, capacity)

	for i := range capacity {
		require.Equal(t, http.StatusOK, openInstance(t, v),
			"stream %d of %d the server may hold", i, capacity)
	}
	require.Equal(t, http.StatusServiceUnavailable, openInstance(t, v),
		"the stream past the cap was served instead of refused")
}

// TestStreamInitRetriesOnRefusal tests that the stream init retries the 503 a
// stream connect past MaxConcurrentInstances receives. Datastar's default
// policy retries network errors only, which leaves a tab refused at the cap
// without a stream until the visitor reloads. "always" retries a status,
// and an unlimited count keeps the tab trying after the default 10 attempts.
func TestStreamInitRetriesOnRefusal(t *testing.T) {
	page := serve(t).Visitor(t).Load(t, "/").Page()
	require.Contains(t, page.Body,
		`data-init="@get('/_$/',{retry:'always',retryMaxCount:Infinity})"`,
		"the stream init carries no retry policy, which leaves a tab refused "+
			"at the instance cap without a stream")
}

// TestInstanceCapIsPerServer covers 2 servers built from 1 generated
// package in 1 process. Each holds its own counter and its own instance store,
// which is what keeps one server's budget and state its own.
//
// Sharing a counter would let a full server A refuse every stream of an
// empty server B; sharing a store would let B serve A's tab.
func TestInstanceCapIsPerServer(t *testing.T) {
	a, b := capServer(t, 1), capServer(t, 1)

	require.Equal(t, http.StatusOK, openInstance(t, a),
		"the only instance server A may hold")
	require.Equal(t, http.StatusServiceUnavailable, openInstance(t, a),
		"a second instance on a server that may hold 1")

	require.Equal(t, http.StatusOK, openInstance(t, b),
		"server B refuses its first stream while server A is full")
}

// TestInstanceIDIsNotSharedBetweenServers tests an id minted by one server and
// presented to another built from the same package. Both accept its shape,
// but only the server whose stream opened it holds its state. The other server
// returns the reconnect response.
func TestInstanceIDIsNotSharedBetweenServers(t *testing.T) {
	a := serve(t).Visitor(t)
	b := serve(t).Visitor(t)

	tab := a.Load(t, "/")
	st := tab.Connect(t, nil)
	require.Equal(t, http.StatusOK, update(t, tab, "on-a").Status,
		"the action on the server that minted the id")

	resp := postUpdate(t, b, tab.Instance(), "on-b")
	require.Equal(t, http.StatusConflict, resp.Status,
		"the other server served a tab it never opened")
	require.Equal(t, "reconnect", resp.Retry())

	// The refusal leaves the tab's state as it was: its next update counts a second
	// delivery.
	require.Equal(t, http.StatusOK, update(t, tab, "again").Status)
	st.WaitFor(t, status("deliveries:2 filter:again"))
}

// TestInstanceCapDisabled covers a negative MaxConcurrentInstances,
// which removes the cap.
//
// The check the cap performs compares the live count against the configured one,
// which a negative value inverts: read literally, a server that may hold
// fewer than 0 instances refuses the first stream and every stream after it.
func TestInstanceCapDisabled(t *testing.T) {
	const streams = 8

	v := capServer(t, -1)

	for i := range streams {
		require.Equal(t, http.StatusOK, openInstance(t, v),
			"stream %d of a server configured to hold as many as it is asked for", i)
	}
}

// TestFailedOpenReleasesInstance covers a stream whose open hook fails after
// its instance was already reserved.
//
// The close hook that gives an instance back is wired up only once the open
// hook has succeeded, which leaves an open that fails to release what it took
// by itself. An instance it kept instead would be held for the life of the process,
// and enough of those leave no tab of any stateful page able to open a stream again.
// The server holds a budget of 1, which is what makes the release observable:
// a second stream is served only if the first gave its instance back.
// The id that named it stops being answered in the same place, and both are asserted.
func TestFailedOpenReleasesInstance(t *testing.T) {
	srv := serve(t, datapages.WithStateConfig(datapages.StateConfig{
		MaxConcurrentInstances: 1,
	}))
	v := srv.Visitor(t)

	page := v.Load(t, pathFailOpen)
	id := page.Instance()
	require.NotEmpty(t, id, "GET %s mints no instance id", pathFailOpen)

	// The instance is reserved before the hook.
	// The stream commits 200 before the hook can fail.
	resp := v.Send(t, page.NewRequest(t, http.MethodGet, pathFailOpenStream, nil))
	require.Equal(t, http.StatusOK, resp.Status,
		"the stream of a page whose open fails")

	// PageIndex holds the same state type. Its action is what asks this
	// server whether the instance of the failed open is still there.
	// An instance that is never given back means its release is wired up only for an
	// open that succeeded.
	srv.WaitFor(t, "the instance of the failed open to be given back", func() bool {
		return postUpdate(t, v, id, "x").Status == http.StatusConflict
	})

	// Nothing else was disturbed: a page whose open succeeds still opens and
	// still holds state. It is also the only instance this server may hold,
	// which a failed open that kept its own would have spent.
	tab := v.Load(t, "/")
	st := tab.Connect(t, nil)
	require.Equal(t, http.StatusOK, update(t, tab, "after-failure").Status)
	st.WaitFor(t, status("deliveries:1 filter:after-failure"))
}

// TestPanicInEventHandlerReleasesInstance covers an OnXXX handler that panics
// while it holds the slot mutex. The event stream ends, and its close path must
// acquire that mutex before it can release the state and its capacity slot.
func TestPanicInEventHandlerReleasesInstance(t *testing.T) {
	// net/http also recovers this panic, but logs its stack.
	// Recover at the test boundary to keep an expected panic out of the test output.
	containPanic := datapages.WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() { _ = recover() }()
			next.ServeHTTP(w, r)
		})
	})
	srv := serve(t, containPanic, datapages.WithStateConfig(datapages.StateConfig{
		MaxConcurrentInstances: 1,
	}))
	v := srv.Visitor(t)

	tab := v.Load(t, "/")
	tab.Connect(t, nil)
	resp := tab.Do(t, http.MethodPost, pathAction, map[string]bool{"panic": true})
	require.Equal(t, http.StatusOK, resp.Status, "dispatching the event that panics")

	// A handler that kept its mutex locked keeps the capacity slot it holds.
	id := v.Load(t, "/").Instance()
	require.NotEmpty(t, id, "GET / mints no replacement instance id")
	srv.WaitFor(t, "the instance capacity slot to be released", func() bool {
		return openStream(t, v, id) == http.StatusOK
	})
}

// TestPanicInStreamCloseIsContained covers a StreamClose that panics.
//
// The hook runs on the watchdog goroutine, which is not one net/http recovers,
// and it holds the slot mutex while it runs. An unrecovered panic there ends
// the process for every user of the server. A recovery that skips the unlock
// and the release wedges the tab instead and keeps its instance forever.
func TestPanicInStreamCloseIsContained(t *testing.T) {
	srv := serve(t)
	v := srv.Visitor(t)

	tab := v.Load(t, pathPanicClose)
	tab.Connect(t, nil).Close()

	require.Equal(t, http.StatusOK, v.Get(t, pathPanicClose).Status,
		"the server stopped serving after a panicking close hook")

	// PageIndex holds the same state type, which is what lets this ask whether
	// the instance the panicking hook held was given back. A mutex the hook
	// never unlocked shows up as an action that never returns. The deadline on
	// the probe turns that into a failure with a message. It cannot keep the
	// case quick, since closing the test server waits on the wedged handler.
	probe := func() int {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		req := updateRequest(t, v, tab.Instance(), "x").WithContext(ctx)
		resp, err := v.Client().Do(req)
		require.NoError(t, err,
			"the action never returned, which is the slot mutex still held "+
				"by the hook that panicked under it")
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	srv.WaitFor(t, "the instance of the panicking close hook to be given back",
		func() bool { return probe() == http.StatusConflict })
}

// updateRequest builds the action as the tab named by instanceID,
// which is how a test reaches the server as a tab no page load of its own minted.
func updateRequest(
	t *testing.T, v *datapagestest.Visitor, instanceID, filter string,
) *http.Request {
	t.Helper()
	req := v.NewRequest(t, http.MethodPost, pathAction,
		strings.NewReader(`{"filter":"`+filter+`"}`))
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Datapages-Instance", instanceID)
	return req
}

func postUpdate(
	t *testing.T, v *datapagestest.Visitor, instanceID, filter string,
) *datapagestest.Response {
	t.Helper()
	return v.Send(t, updateRequest(t, v, instanceID, filter))
}

// streamRequest builds the request that opens
// the index page's stream as the tab named by instanceID.
func streamRequest(
	t *testing.T, v *datapagestest.Visitor, instanceID string,
) *http.Request {
	t.Helper()
	req := v.NewRequest(t, http.MethodGet, pathStream, nil)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Datapages-Instance", instanceID)
	return req
}

// openStream connects a stream under instanceID and closes it after its status arrives.
// A refused stream returns immediately; an accepted SSE stream ends
// when its body is closed.
func openStream(t *testing.T, v *datapagestest.Visitor, instanceID string) int {
	t.Helper()
	resp, err := v.Client().Do(streamRequest(t, v, instanceID))
	require.NoError(t, err, "opening the stream")
	_ = resp.Body.Close()
	return resp.StatusCode
}
