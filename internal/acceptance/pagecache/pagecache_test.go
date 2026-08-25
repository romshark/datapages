// Covers the page cache: what a handler reads through the pageCache parameter,
// and what the generated server sends the service worker for each kind of handler.
//
// The delivered payload is JSON inside a script. A test asserts on what the
// worker acts on: the message type, the URL of an entry and its version.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/pagecache/app"
	"github.com/romshark/datapages/internal/acceptance/pagecache/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/offline"
)

// applyType is the message the worker acts on.
// Nothing else it receives writes the cache.
const applyType = `"type":"datapages-offline:apply"`

// workerVersion is what the case installs the worker as. Any non-zero value does;
// the tests read it back from the worker script and from what the middleware decides.
const workerVersion = 42

func newClient(t *testing.T, opts ...datapages.ServerOption) *client.Client {
	t.Helper()
	return client.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer), opts...))
}

func TestGETBakesQueuedWrites(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, "/")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "0", resp.Element(t, "held"),
		"no header means nothing is cached")
	require.Contains(t, resp.Body, applyType,
		"a GET bakes what it queued into the page")
	require.Contains(t, resp.Body, `"url":"/"`)
	require.Contains(t, resp.Body, `"version":7`)
	require.Contains(t, resp.Body, `index offline`,
		"the entry carries the rendered body, not the live one")
}

func TestGETReadsTheHeldVersion(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	req := c.Request(t, http.MethodGet, "/", "")
	req.Header.Set("X-Datapages-Offline-Version", "7")
	resp := c.Do(t, req)

	require.Equal(t, "7", resp.Element(t, "held"),
		"the handler reads the version the worker reports")
	require.NotContains(t, resp.Body, applyType,
		"the client holds the current copy, so the handler queues nothing")
}

func TestGETWritesNothingWhenNothingIsQueued(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, "/offline/")
	require.Equal(t, http.StatusOK, resp.Status)
	require.NotContains(t, resp.Body, applyType,
		"a page that never touches the cache carries no script")
}

func TestActionFlushesOverItsStream(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Action(t, http.MethodPost, "/stream-write/", "")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, resp.Body, applyType,
		"an action with a stream delivers over it")
	require.Contains(t, resp.Body, `"version":2`)
	require.Contains(t, resp.Body, "done", "the action's own patch is sent too")
}

func TestRedirectCarriesWritesBeforeNavigating(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Action(t, http.MethodPost, "/redirect-write/", "")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/javascript")
	require.Contains(t, resp.Body, applyType)
	require.Contains(t, resp.Body, `"clearAll":true`)

	require.Contains(t, resp.Body, "postMessage(",
		"the writes go to the worker from the redirect response")
	require.Contains(t, resp.Body, `function(){window.location="/";}`,
		"the navigation is deferred behind a function, which is what keeps the"+
			" writes from being lost to the unload")
}

func TestUnclaimedURLBakesQueuedWrites(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	// The server canonicalizes the URL before the page sees it.
	resp := c.Get(t, "/no-page-claims-this")
	require.Equal(t, http.StatusNotFound, resp.Status)
	require.Equal(t, "/no-page-claims-this/", resp.Element(t, "missing"))
	require.Contains(t, resp.Body, applyType,
		"the inline 404 render delivers what its handler queued")
	require.Contains(t, resp.Body, `"clears":["/no-page-claims-this/"]`)
}

func TestWithOfflineServesTheWorker(t *testing.T) {
	t.Parallel()
	c := newClient(t, datapagesgen.WithOffline(
		offline.Config{WorkerVersion: workerVersion}))

	resp := c.Get(t, offline.DefaultScriptURL)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "/", resp.Header.Get("Service-Worker-Allowed"),
		"the worker controls the whole origin, not the path it is served from")
	require.Contains(t, resp.Body, `"offlineURL":"/offline/"`,
		"the route of PageOffline reaches the worker without being configured")
	require.Contains(t, resp.Body, `"workerVersion":42`)
}

func TestWithOfflineRegistersOnlyWhileOutdated(t *testing.T) {
	t.Parallel()
	c := newClient(t, datapagesgen.WithOffline(
		offline.Config{WorkerVersion: workerVersion}))

	resp := c.Get(t, "/")
	require.Contains(t, resp.Body, "serviceWorker.register(",
		"a client with no worker is sent the registration script")

	req := c.Request(t, http.MethodGet, "/", "")
	req.Header.Del("Datastar-Request")
	req.Header.Set("X-Datapages-Worker-Version", "42")
	current := c.Do(t, req)
	require.NotContains(t, current.Body, "serviceWorker.register(",
		"a client running the current worker is sent no registration script")
}
