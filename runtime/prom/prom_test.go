package prom_test

import (
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

func TestMiddleware(t *testing.T) {
	h := prom.Middleware(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/thing/", nil))

	require.Equal(t, http.StatusTeapot, w.Code)
	require.Contains(t, gather(t, "datapages_http_requests_total"), "418")
	require.Contains(t, gather(t, "datapages_http_requests_total"), "/thing/")
}

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

// TestAuthMetricsImplementsAuth covers that the counters satisfy what the
// session manager asks for.
func TestAuthMetricsImplementsAuth(t *testing.T) {
	var m auth.Metrics = prom.AuthMetrics{}
	m.SessionRead("valid")
	m.SessionCreated("success")
	m.SessionClosed("success")
	require.Contains(t, gather(t, "datapages_session_reads_total"), "valid")
}
