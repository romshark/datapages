package prom_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/runtime/prom"
)

// registry is shared. The metrics are package-level and register once,
// which a second registry would come up empty for.
var registry = func() *prometheus.Registry {
	r := prometheus.NewRegistry()
	prom.Register(r)
	return r
}()

func TestRegisterOnce(t *testing.T) {
	// A second call must not panic on a duplicate collector.
	prom.Register(prometheus.NewRegistry())
	prom.Register(registry)
}

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
