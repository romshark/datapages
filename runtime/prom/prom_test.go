package prom_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/runtime/auth"
	"github.com/romshark/datapages/runtime/prom"
)

// registry is shared by the assertions below, which read what the metrics of
// this package recorded. Registering on one of its own is covered by
// register_test.go.
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

	require.Contains(t, gather(t, "datapages_sse_disconnects_total"), "client")
	require.Contains(t, gather(t, "datapages_session_reads_total"), "valid")
	require.Contains(t, gather(t, "datapages_session_creations_total"), "success")
	require.Contains(t, gather(t, "datapages_session_closures_total"), "error")
	require.Contains(t, gather(t, "datapages_event_broker_publishes_by_kind_total"), "public")
	require.NotEmpty(t, gather(t, "datapages_internal_errors_recovered_total"))
	require.NotEmpty(t, gather(t, "datapages_sse_connection_duration_seconds"))
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
