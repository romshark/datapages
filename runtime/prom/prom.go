// Package prom holds the metrics of a generated server,
// their registration and the middleware that fills them.
//
// Application code must not import this package.
package prom

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	mHTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "datapages",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
	mHTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "datapages",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP request latency",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	mInternalErrorsRecovered = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "datapages",
			Name:      "internal_errors_recovered_total",
			Help:      "Internal errors recovered without HTTP failure",
		},
	)
	mInternalErrorsNotRecovered = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "datapages",
			Name:      "internal_errors_not_recovered_total",
			Help: "Internal errors that could not be recovered and " +
				"resulted in an HTTP error response",
		},
	)
	mInFlightRequests = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "datapages",
			Subsystem: "http",
			Name:      "in_flight_requests",
			Help:      "Current in-flight HTTP requests",
		},
	)

	mSSEConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "datapages",
			Subsystem: "http",
			Name:      "sse_connections",
			Help:      "Active SSE connections",
		},
	)

	// By-kind (low-cardinality) publish counters.
	// Useful because subjects include user IDs (high-cardinality),
	// so we collapse to event kinds.
	mBrokerEventPublishes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "datapages",
			Subsystem: "event_broker",
			Name:      "publishes_by_kind_total",
			Help:      "Published events by kind (low-cardinality)",
		},
		[]string{"kind"},
	)

	// Dropped broker deliveries (slow consumers).
	mBrokerDeliveriesDropped = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "datapages",
			Subsystem: "event_broker",
			Name:      "deliveries_dropped_total",
			Help:      "Dropped broker deliveries due to slow consumers",
		},
	)

	// SSE connection lifetime + disconnect reasons.
	mSSEConnectionDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "datapages",
			Subsystem: "sse",
			Name:      "connection_duration_seconds",
			Help:      "SSE connection lifetime in seconds",
			Buckets:   prometheus.DefBuckets,
		},
	)

	mSSEDisconnects = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "datapages",
			Subsystem: "sse",
			Name:      "disconnects_total",
			Help:      "SSE disconnects by reason",
		},
		[]string{"reason"}, // "ttl" | "client" | "shutdown"
	)

	mSessionCreations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "datapages",
			Subsystem: "session",
			Name:      "creations_total",
			Help:      "Session creations",
		},
		[]string{"result"}, // "success" | "error"
	)
	mSessionClosures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "datapages",
			Subsystem: "session",
			Name:      "closures_total",
			Help:      "Session closures",
		},
		[]string{"result"}, // "success" | "error"
	)
	mSessionReads = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "datapages",
			Subsystem: "session",
			Name:      "reads_total",
			Help:      "Session reads from cookie",
		},
		[]string{"result"}, // "valid" | "none" | "stale" | "expired" | "error"
	)
)

var registerOnce sync.Once

// Register registers the built-in metrics on r, once per process.
func Register(r prometheus.Registerer) {
	registerOnce.Do(func() {
		r.MustRegister(
			mHTTPRequestsTotal,
			mHTTPRequestDuration,
			mInFlightRequests,
			mInternalErrorsRecovered,
			mInternalErrorsNotRecovered,
			mSSEConnections,
			mSSEConnectionDuration,
			mSSEDisconnects,
			mBrokerEventPublishes,
			mBrokerDeliveriesDropped,
			mSessionCreations,
			mSessionClosures,
			mSessionReads,
		)
	})
}

// SSEConnectionOpened counts a stream the server just accepted.
func SSEConnectionOpened() { mSSEConnections.Inc() }

// SSEConnectionClosed counts down the stream the server just let go.
func SSEConnectionClosed() { mSSEConnections.Dec() }

// SSEDisconnect counts why a stream ended.
// reason is "close", "client" or "shutdown".
func SSEDisconnect(reason string) {
	mSSEDisconnects.WithLabelValues(reason).Inc()
}

// SSEConnectionDuration records how long a stream was open.
func SSEConnectionDuration(since time.Time) {
	mSSEConnectionDuration.Observe(time.Since(since).Seconds())
}

// SessionRead counts a session lookup. outcome is "none", "error", "stale",
// "expired" or "valid".
func SessionRead(outcome string) {
	mSessionReads.WithLabelValues(outcome).Inc()
}

// SessionCreated counts a session issued. outcome is "success" or "error".
func SessionCreated(outcome string) {
	mSessionCreations.WithLabelValues(outcome).Inc()
}

// SessionClosed counts a session removed. outcome is "success" or "error".
func SessionClosed(outcome string) {
	mSessionClosures.WithLabelValues(outcome).Inc()
}

// InternalErrorRecovered counts an error the application answered itself.
func InternalErrorRecovered() { mInternalErrorsRecovered.Inc() }

// InternalErrorNotRecovered counts an error the generated server answered.
func InternalErrorNotRecovered() { mInternalErrorsNotRecovered.Inc() }

// AuthMetrics counts what the session manager does.
// It implements auth.Metrics.
type AuthMetrics struct{}

func (AuthMetrics) SessionRead(outcome string)    { SessionRead(outcome) }
func (AuthMetrics) SessionCreated(outcome string) { SessionCreated(outcome) }
func (AuthMetrics) SessionClosed(outcome string)  { SessionClosed(outcome) }

// BrokerPublish counts an event published, by the kind of its subject.
func BrokerPublish(subjectKind string) {
	mBrokerEventPublishes.WithLabelValues(subjectKind).Inc()
}

// BrokerDeliveryDropped counts an event a subscriber never received.
func BrokerDeliveryDropped() { mBrokerDeliveriesDropped.Inc() }

// statusRW records the status code the handler wrote.
type statusRW struct {
	http.ResponseWriter
	status int
}

func (w *statusRW) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRW) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusRW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("http.Hijacker not supported")
}

func (w *statusRW) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// routeLabel is the route a request matched, or its path when it matched none.
func routeLabel(r *http.Request) string {
	if p := r.Pattern; p != "" {
		return p
	}
	return r.URL.Path
}

// Middleware measures every request. It must be the outermost middleware
// of the chain, otherwise it misses the work of the ones before it.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		mInFlightRequests.Inc()
		defer mInFlightRequests.Dec()

		rw := &statusRW{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		path := routeLabel(r)
		mHTTPRequestsTotal.
			WithLabelValues(r.Method, path, strconv.Itoa(rw.status)).Inc()
		mHTTPRequestDuration.
			WithLabelValues(r.Method, path).Observe(time.Since(start).Seconds())
	})
}
