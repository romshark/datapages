package prom_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/runtime/actionexpr"
	"github.com/romshark/datapages/runtime/auth"
	"github.com/romshark/datapages/runtime/prom"
)

// registry is shared by the assertions below, which read what the metrics of
// this package recorded. Registering on one of its own is covered by register_test.go.
var registry = func() *prometheus.Registry {
	r := prometheus.NewRegistry()
	if err := prom.Register(r); err != nil {
		panic(err)
	}
	return r
}()

func gather(t *testing.T, name string) string {
	t.Helper()
	var b strings.Builder
	metrics, err := registry.Gather()
	require.NoError(t, err)
	for _, m := range metrics {
		if m.GetName() == name {
			b.WriteString(m.String())
		}
	}
	return b.String()
}

// TestMiddleware tests that the request counter records the route pattern and
// the status code, and that wrapping a handler leaves its response alone.
func TestMiddleware(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /thing/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	h := prom.Middleware(mux)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/thing/", nil))

	require.Equal(t, http.StatusTeapot, w.Code)
	require.Contains(t, gather(t, "datapages_http_requests_total"), "418")
	require.Contains(t, gather(t, "datapages_http_requests_total"), "GET /thing/")
}

// TestMiddlewareLabelsTheSentStatus tests a handler that writes a status after
// the body has gone out. net/http sends the first one and drops the second,
// so labelling with the last counts a 5xx no client ever saw.
func TestMiddlewareLabelsTheSentStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /late/{$}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("half a page"))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	})
	h := prom.Middleware(mux)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/late/", nil))

	require.Equal(t, http.StatusOK, w.Code)
	const metric = "datapages_http_requests_total"
	require.Equal(t, 1, series(t, metric, map[string]string{
		"path": "GET /late/{$}", "status": "200",
	}), "the status the client received was not counted")
	require.Zero(t, series(t, metric, map[string]string{
		"path": "GET /late/{$}", "status": "500",
	}), "a status net/http never sent was counted")
}

// TestMiddlewareWriteString tests io.WriteString on the middleware's writer.
// It reaches the WriteString of the writer underneath without copying the
// string into a new byte slice, and a status written after it is dropped,
// as after Write.
func TestMiddlewareWriteString(t *testing.T) {
	var allocs float64
	mux := http.NewServeMux()
	mux.HandleFunc("GET /string/{$}", func(w http.ResponseWriter, r *http.Request) {
		allocs = testing.AllocsPerRun(100, func() {
			_, _ = io.WriteString(w, "half a page")
		})
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	})
	prom.Middleware(mux).ServeHTTP(discardRW{http.Header{}},
		httptest.NewRequest(http.MethodGet, "/string/", nil))

	require.Zero(t, allocs, "allocations per io.WriteString")
	const metric = "datapages_http_requests_total"
	require.Equal(t, 1, series(t, metric, map[string]string{
		"path": "GET /string/{$}", "status": "200",
	}), "the status the client received was not counted")
	require.Zero(t, series(t, metric, map[string]string{
		"path": "GET /string/{$}", "status": "500",
	}), "a status net/http never sent was counted")
}

// discardRW drops the body. Like the writer net/http hands a handler,
// it has a WriteString that copies nothing.
type discardRW struct{ h http.Header }

func (w discardRW) Header() http.Header             { return w.h }
func (discardRW) Write(p []byte) (int, error)       { return len(p), nil }
func (discardRW) WriteString(s string) (int, error) { return len(s), nil }
func (discardRW) WriteHeader(int)                   {}

// TestMarkStreamLeavesRequestLatency tests a request the stream handler marked:
// it leaves the latency histogram and the in-flight gauge, and stays counted.
func TestMarkStreamLeavesRequestLatency(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /stream/{$}", func(w http.ResponseWriter, r *http.Request) {
		prom.MarkStream(w)
		requireInFlight(t, "0")
	})
	mux.HandleFunc("GET /page/{$}", func(w http.ResponseWriter, r *http.Request) {
		requireInFlight(t, "1")
	})
	h := prom.Middleware(mux)

	h.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/stream/", nil))
	h.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/page/", nil))

	got := gather(t, "datapages_http_request_duration_seconds")
	require.NotContains(t, got, "GET /stream/{$}",
		"the stream was observed as a request latency:\n%s", got)
	require.Contains(t, got, "GET /page/{$}", "the page was not observed")

	total := gather(t, "datapages_http_requests_total")
	require.Contains(t, total, "GET /stream/{$}", "the stream was not counted")
	requireInFlight(t, "0")
}

// TestMiddlewareCountsAbortedStream tests a stream that shutdown ends with an
// [http.ErrAbortHandler] panic. The stream stays counted, and the panic still
// reaches net/http, which resets the connection.
func TestMiddlewareCountsAbortedStream(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /aborted/{$}", func(w http.ResponseWriter, r *http.Request) {
		prom.MarkStream(w)
		panic(http.ErrAbortHandler)
	})
	h := prom.Middleware(mux)

	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.ServeHTTP(httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "/aborted/", nil))
	})
	require.Equal(t, 1, series(t, "datapages_http_requests_total", map[string]string{
		"path": "GET /aborted/{$}", "status": "200",
	}), "the aborted stream was not counted")
	requireInFlight(t, "0")
}

type nonceKey struct{}

// withNonce stores a value in the request context, as an application storing
// a CSP nonce does. The next handler receives a copy of the request.
func withNonce(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), nonceKey{}, "nonce")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TestRecordRoute tests a middleware between Middleware and the router that
// derives the request. The router sets the route on the copy, and both the
// request and a stream that shutdown aborts keep their route label.
func TestRecordRoute(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /derived/{id}/", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("GET /derived-stream/{$}", func(w http.ResponseWriter, r *http.Request) {
		prom.MarkStream(w)
		panic(http.ErrAbortHandler)
	})
	h := prom.Middleware(withNonce(prom.RecordRoute(mux)))

	h.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/derived/1/", nil))
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		h.ServeHTTP(httptest.NewRecorder(),
			httptest.NewRequest(http.MethodGet, "/derived-stream/", nil))
	})

	const total = "datapages_http_requests_total"
	require.Equal(t, 1, series(t, total, map[string]string{
		"path": "GET /derived/{id}/", "status": "200",
	}), "the request was not counted under its route")
	require.Equal(t, 1, series(t, "datapages_http_request_duration_seconds",
		map[string]string{"path": "GET /derived/{id}/"},
	), "the request latency was not observed under its route")
	require.Equal(t, 1, series(t, total, map[string]string{
		"path": "GET /derived-stream/{$}",
	}), "the aborted stream was not counted under its route")
	requireInFlight(t, "0")
}

// requireInFlight requires the in-flight gauge to read want.
func requireInFlight(t *testing.T, want string) {
	t.Helper()
	got := gather(t, "datapages_http_in_flight_requests")
	require.Contains(t, got, "value:"+want, "in-flight gauge:\n%s", got)
}

// TestMiddlewareLabelsAreBounded tests the cardinality of the HTTP metrics.
// Both labels come from a closed set: the routes the router registers,
// plus one label for everything else.
func TestMiddlewareLabelsAreBounded(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /user/{uid}/", func(http.ResponseWriter, *http.Request) {})
	h := prom.Middleware(mux)

	send := func(method, target string) {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, target, nil))
	}
	for i := range 5 {
		send(http.MethodGet, fmt.Sprintf("/user/%d/", i))
		send(http.MethodGet, fmt.Sprintf("/nothing/%d/", i))
		send(fmt.Sprintf("BOGUS%d", i), "/user/1/")
	}

	const metric = "datapages_http_requests_total"
	require.Equal(t, 1, series(t, metric, map[string]string{
		"path": "GET /user/{uid}/", "status": "200",
	}), "one series for every user of the route")
	require.Equal(t, 1, series(t, metric, map[string]string{
		"path": prom.LabelUnmatched, "status": "404",
	}), "one series for every path that matched no route")
	require.Equal(t, 1, series(t, metric, map[string]string{
		"method": prom.LabelOtherMethod,
	}), "one series for every method no route registers")
	require.Zero(t, series(t, metric, map[string]string{
		"path": "/user/1/",
	}), "the raw path must not appear as a label")
}

// TestMiddlewareLabelsQuery tests a QUERY request, which an action can answer.
// It keeps its method as the label instead of [prom.LabelOtherMethod].
func TestMiddlewareLabelsQuery(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("QUERY /search/", func(http.ResponseWriter, *http.Request) {})
	h := prom.Middleware(mux)

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("QUERY", "/search/", nil))

	require.Equal(t, 1, series(t, "datapages_http_requests_total", map[string]string{
		"method": "QUERY", "path": "QUERY /search/",
	}))
}

// series counts the series of the metric family name carrying every label of want.
func series(t *testing.T, name string, want map[string]string) int {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	count := 0
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			got := map[string]string{}
			for _, l := range m.GetLabel() {
				got[l.GetName()] = l.GetValue()
			}
			match := true
			for k, v := range want {
				if got[k] != v {
					match = false
					break
				}
			}
			if match {
				count++
			}
		}
	}
	return count
}

// TestCounters tests that every counter this package exports reaches the
// registry under the metric name and the label value it was called with.
func TestCounters(t *testing.T) {
	prom.SSEConnectionOpened()
	prom.SSEDisconnect("client")
	prom.SSEConnectionDuration(time.Now())
	prom.SSEConnectionClosed()
	prom.SessionRead("valid")
	prom.SessionCreated("success")
	prom.SessionClosed("error")
	prom.InternalErrorRecovered()
	prom.InternalErrorNotRecovered()
	prom.BrokerPublish("public")
	prom.BrokerDeliveryDropped()
	prom.ActionOptionDropped("WithRetryScaler")

	require.Contains(t, gather(t, "datapages_sse_disconnects_total"), "client")
	require.Contains(t, gather(t, "datapages_session_reads_total"), "valid")
	require.Contains(t, gather(t, "datapages_session_creations_total"), "success")
	require.Contains(t, gather(t, "datapages_session_closures_total"), "error")
	require.Contains(t, gather(t, "datapages_event_broker_publishes_by_kind_total"), "public")
	require.Contains(t, gather(t, "datapages_action_options_dropped_total"),
		"WithRetryScaler")
	require.NotEmpty(t, gather(t, "datapages_internal_errors_recovered_total"))
	require.NotEmpty(t, gather(t, "datapages_sse_connection_duration_seconds"))
}

// TestActionMetricsImplementsActionexpr tests that the counters satisfy what
// the action expression writer asks for.
func TestActionMetricsImplementsActionexpr(t *testing.T) {
	var m actionexpr.Metrics = prom.ActionMetrics{}
	m.OptionDropped("WithRetry")
	require.Contains(t, gather(t, "datapages_action_options_dropped_total"),
		"WithRetry")
}

// TestAuthMetricsImplementsAuth tests that the counters satisfy what the
// session manager asks for.
func TestAuthMetricsImplementsAuth(t *testing.T) {
	var m auth.Metrics = prom.AuthMetrics{}
	m.SessionRead("valid")
	m.SessionCreated("success")
	m.SessionClosed("success")
	require.Contains(t, gather(t, "datapages_session_reads_total"), "valid")
}
