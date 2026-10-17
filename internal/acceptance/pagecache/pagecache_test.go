package acceptance_test

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/pagecache/app"
	"github.com/romshark/datapages/internal/acceptance/pagecache/app/datapagesgen"
	"github.com/romshark/datapages/internal/testlog"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/offline"
)

// applyType identifies the only message that changes the worker cache.
const applyType = `"type":"datapages-offline:apply"`

// workerVersion is the value asserted in worker and middleware responses.
const workerVersion = 42

func newClient(t *testing.T, opts ...datapages.ServerOption) *client.Client {
	t.Helper()
	return client.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer), opts...))
}

// TestGETEmbedsQueuedWrites tests that a page load embeds its queued writes.
// The version is a JSON string, which JavaScript reads without rounding above 2^53.
func TestGETEmbedsQueuedWrites(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, "/")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "0", resp.Element(t, "held"),
		"no header means nothing is cached")
	require.Contains(t, resp.Body, applyType,
		"a GET embeds queued writes in the page")
	require.Contains(t, resp.Body, `"url":"/"`)
	require.Contains(t, resp.Body, `"version":"9007199254740993"`)
	require.Contains(t, resp.Body, `index offline`,
		"the entry carries the rendered body, not the live one")
}

// TestCachedBodyWriterTakesStrings tests that the writer a cached body renders
// into has WriteString. The document frame of the entry is written to it with
// io.WriteString, which copies each string into a new slice without it.
func TestCachedBodyWriterTakesStrings(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, "/writer-probe")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, resp.Body, applyType)
	require.Contains(t, resp.Body, "string writer: true")
}

func TestGETReadsTheHeldVersion(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	req := c.Request(t, http.MethodGet, "/", "")
	req.Header.Set("X-Datapages-Offline-Version", "9007199254740993")
	resp := c.Do(t, req)

	require.Equal(t, "9007199254740993", resp.Element(t, "held"),
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
	require.Contains(t, resp.Body, `"version":"2"`)
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
	require.Contains(t, resp.Body, "ch.port1.onmessage=once",
		"the navigation waits for the worker to acknowledge the apply")
	require.Contains(t, resp.Body, "setTimeout(once,500)",
		"a worker too old to acknowledge does not strand the navigation")
}

func TestUnclaimedURLEmbedsQueuedWrites(t *testing.T) {
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
		offline.Config{WorkerVersion: workerVersion},
	))

	resp := c.Get(t, offline.DefaultScriptURL)
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "/", resp.Header.Get("Service-Worker-Allowed"),
		"the worker controls the whole origin, not the path it is served from")
	require.Contains(t, resp.Body, `"offlineURL":"/offline/"`,
		"the route of PageOffline reaches the worker without being configured")
	require.Contains(t, resp.Body, `"workerVersion":"42"`)
}

func TestWithOfflineRegistersOnlyWhileOutdated(t *testing.T) {
	t.Parallel()
	c := newClient(t, datapagesgen.WithOffline(
		offline.Config{WorkerVersion: workerVersion},
	))

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

func TestAppLevelActionFlushesOverAStream(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Action(t, http.MethodPost, "/app-precache/", "")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
	require.Contains(t, resp.Body, applyType,
		"an app-level action with neither a redirect nor a body"+
			" delivers over the stream the framework opens for it")
	require.Contains(t, resp.Body, `"url":"/"`)
	require.Contains(t, resp.Body, `"version":"11"`)
	require.Contains(t, resp.Body, "written by an app-level action",
		"the entry carries the rendered body")
}

// TestAppLevelActionStreamConfig tests an App action's static and
// request-scoped StreamConfig.
func TestAppLevelActionStreamConfig(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	for name, tc := range map[string]struct {
		path     string
		saveData bool
		coding   string
	}{
		"server's":  {path: "/app-precache/", coding: "zstd"},
		"save data": {path: "/app-precache/", saveData: true, coding: "gzip"},
		"static":    {path: "/app-precache-fail/"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequestWithContext(context.Background(),
				http.MethodPost, c.URL()+tc.path, nil)
			require.NoError(t, err, "building POST %s", tc.path)
			req.Header.Set("Datastar-Request", "true")
			req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
			if tc.saveData {
				req.Header.Set("Save-Data", "on")
			}
			resp, err := c.HTTP().Do(req)
			require.NoError(t, err, "POST %s", tc.path)
			_ = resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")
			require.Equal(t, tc.coding, resp.Header.Get("Content-Encoding"))
		})
	}
}

func TestAppLevelActionEmbedsWritesInItsBody(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Action(t, http.MethodPost, "/app-body/", "")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/html")
	require.Equal(t, "done", resp.Element(t, "out"))
	require.Contains(t, resp.Body, applyType,
		"an app-level action embeds writes in its document")
	require.Contains(t, resp.Body, `"version":"12"`)
}

func TestStreamRedirectFlushesBeforeNavigating(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Action(t, http.MethodPost, "/stream-redirect-write/", "")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, resp.Body, applyType,
		"an action redirecting through its own stream delivers over it first")
	require.Contains(t, resp.Body, `"version":"14"`)
	require.Less(t, strings.Index(resp.Body, applyType),
		strings.Index(resp.Body, "window.location.href"),
		"the writes reach the worker ahead of the navigation")
}

// TestEmbeddedWritesSitInsideTheBody tests that every HTML response puts its
// cache script inside the body. Shim updates replace only the body element.
func TestEmbeddedWritesSitInsideTheBody(t *testing.T) {
	t.Parallel()
	for name, get := range map[string]func(*testing.T, *client.Client) string{
		"get": func(t *testing.T, c *client.Client) string {
			return c.Get(t, "/").Body
		},
		"inline 404": func(t *testing.T, c *client.Client) string {
			return c.Get(t, "/no-page-claims-this").Body
		},
		"app action body": func(t *testing.T, c *client.Client) string {
			return c.Action(t, http.MethodPost, "/app-body/", "").Body
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := get(t, newClient(t))
			require.Contains(t, body, applyType)
			require.Less(t, strings.Index(body, applyType),
				strings.LastIndex(body, "</body>"))
		})
	}
}

// TestShimKeepsItsQuery tests that a shim is cached under the full URL and
// asks for its live page with the query. Keying on the path alone would answer
// /list?page=2 from the entry for /list.
func TestShimKeepsItsQuery(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, "/list/?page=2")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "2", resp.Element(t, "page"))
	require.Contains(t, resp.Body, `"url":"/list/?page=2"`)
	require.Contains(t, resp.Body, `"shim":true`)
	require.Contains(t, resp.Body,
		"window.location.pathname+window.location.search")
	require.Contains(t, resp.Body, `X-Datapages-Shim-Hydrate`,
		"the worker recognises the hydration request by header, which is what"+
			" lets it answer one whose prefetch it lost")
}

// TestPageCacheVariesByHeldVersion tests Vary on page and stream responses.
func TestPageCacheVariesByHeldVersion(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	require.Contains(t,
		c.Get(t, "/").Header.Values("Vary"),
		"X-Datapages-Offline-Version")
	// Its response compression varies by Accept-Encoding.
	require.NotContains(t,
		c.Get(t, "/offline/").Header.Values("Vary"), "X-Datapages-Offline-Version",
		"a page that never touches the cache does not vary by its version")
	require.Contains(t,
		c.Action(t, http.MethodPost, "/stream-write/", "").Header.Values("Vary"),
		"X-Datapages-Offline-Version")
}

// TestFailingCachedBodyIsLogged tests render failures in action delivery paths.
func TestFailingCachedBodyIsLogged(t *testing.T) {
	t.Parallel()
	for name, path := range map[string]string{
		"own stream": "/stream-write-fail/",
		"new stream": "/app-precache-fail/",
		"redirect":   "/redirect-write-fail/",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var log testlog.Buffer
			c := newClient(t, datapages.WithLogger(log.Logger()))

			resp := c.Action(t, http.MethodPost, path, "")
			require.Equal(t, http.StatusOK, resp.Status)
			require.NotContains(t, resp.Body, applyType)
			log.RequireLogged(t, `level=ERROR msg="rendering page cache writes" `+
				`err="rendering page cache body for /: the cached body failed"`)
		})
	}
}

// TestBranchingActionDeliversOnBothPaths tests an action that picks between a
// body and a redirect at run time. Both response paths must send queued writes.
func TestBranchingActionDeliversOnBothPaths(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		path        string
		contentType string
	}{
		"body":     {"/branch/", "text/html"},
		"redirect": {"/branch/?redirect=1", "text/javascript"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := newClient(t).Action(t, http.MethodPost, tc.path, "")
			require.Equal(t, http.StatusOK, resp.Status)
			require.Contains(t, resp.Header.Get("Content-Type"), tc.contentType)
			require.Contains(t, resp.Body, applyType)
			require.Contains(t, resp.Body, `"version":"15"`)
		})
	}
}

// TestPageCacheActionsRefusePlainRequests tests that every action taking a
// [datapages.PageCacheWriter], on a page or on App, answers 406 to a request
// without Datastar-Request. A plain form posting to one that redirects would
// get an HTTP redirect, which drops the queued writes.
func TestPageCacheActionsRefusePlainRequests(t *testing.T) {
	t.Parallel()
	c := newClient(t)
	// The default client follows a redirect, which hides the status.
	hc := &http.Client{
		Transport: c.HTTP().Transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for name, path := range map[string]string{
		"page stream":           "/stream-write/",
		"page redirect":         "/redirect-write/",
		"page stream redirect":  "/stream-redirect-write/",
		"page body or redirect": "/branch/",
		"app stream":            "/app-precache/",
		"app body":              "/app-body/",
		"app redirect":          "/app-redirect/",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			req, err := http.NewRequestWithContext(context.Background(),
				http.MethodPost, c.URL()+path, nil)
			require.NoError(t, err, "building POST %s", path)
			resp, err := hc.Do(req)
			require.NoError(t, err, "POST %s", path)
			_ = resp.Body.Close()
			require.Equal(t, http.StatusNotAcceptable, resp.StatusCode)
		})
	}
}

// TestCSPNonceReachesEveryScript tests that both option orders add the request
// nonce to the page cache and offline middleware scripts.
func TestCSPNonceReachesEveryScript(t *testing.T) {
	t.Parallel()

	const nonce = "t35tN0nce"
	nonceOpt := datapages.WithCSPNonce(func(*http.Request) string { return nonce })
	offlineOpt := datapagesgen.WithOffline(offline.Config{
		WorkerVersion: workerVersion,
	})

	for name, opts := range map[string][]datapages.ServerOption{
		"nonce first":   {nonceOpt, offlineOpt},
		"offline first": {offlineOpt, nonceOpt},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := newClient(t, opts...).Get(t, "/")
			require.Equal(t, http.StatusOK, resp.Status)
			require.Contains(t, resp.Body, applyType)
			require.Contains(t, resp.Body, `<html data-nonce="`+nonce+`">`,
				"data-nonce enables Datastar CSP mode")

			tags := regexp.MustCompile(`<script[^>]*>`).FindAllString(resp.Body, -1)
			require.Len(t, tags, 5,
				"expected 1 build, 1 Datastar, 2 offline, and 1 cache-write script")
			for _, tag := range tags {
				require.Contains(t, tag, `nonce="`+nonce+`"`,
					"inline script requires the request nonce")
			}
		})
	}
}
