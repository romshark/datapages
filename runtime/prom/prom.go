// Package prom holds the metrics of a generated server,
// their registration and the middleware that fills them.
//
// Application code must not import this package.
package prom

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	mHTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "datapages",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total HTTP requests, SSE streams included",
		},
		[]string{"method", "path", "status"},
	)
	mHTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "datapages",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help: "HTTP request latency, excluding SSE streams, " +
				"whose lifetime is in sse_connection_duration_seconds",
			Buckets: prometheus.DefBuckets,
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
			Help: "Current in-flight HTTP requests, excluding SSE streams, " +
				"which are counted in sse_connections",
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
		[]string{"reason"}, // [SSEDisconnect] names the values.
	)

	// Action options the expression writer refused. The label is the helper
	// that built the option, never the value, which can come from a request
	// and would then open one time series per visitor.
	mActionOptionsDropped = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "datapages",
			Subsystem: "action",
			Name:      "options_dropped_total",
			Help:      "Action options dropped for a value the expression cannot carry",
		},
		[]string{"option"},
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

// Register registers the built-in metrics on r, followed by extra.
// A collector r already holds is skipped, which lets two servers of one
// process share a registerer.
func Register(r prometheus.Registerer, extra ...prometheus.Collector) error {
	builtin := [...]prometheus.Collector{
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
		mActionOptionsDropped,
		mSessionCreations,
		mSessionClosures,
		mSessionReads,
	}
	for _, c := range builtin {
		if err := register(r, c); err != nil {
			return err
		}
	}
	for _, c := range extra {
		if err := register(r, c); err != nil {
			return fmt.Errorf("registering collector: %w", err)
		}
	}
	return nil
}

func register(r prometheus.Registerer, c prometheus.Collector) error {
	err := r.Register(c)
	var already prometheus.AlreadyRegisteredError
	if err == nil || errors.As(err, &already) {
		return nil
	}
	return err
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

// ActionOptionDropped counts an action option left out of the expression.
// option is the name of the helper that built it, such as "WithRetryScaler".
func ActionOptionDropped(option string) {
	mActionOptionsDropped.WithLabelValues(option).Inc()
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

// ActionMetrics counts what the action expression writer refuses.
// It implements actionexpr.Metrics.
type ActionMetrics struct{}

func (ActionMetrics) OptionDropped(option string) { ActionOptionDropped(option) }

// StreamMetrics counts what the stream handler does.
// It implements stream.Metrics.
type StreamMetrics struct{}

func (StreamMetrics) ConnectionOpened(w http.ResponseWriter) {
	MarkStream(w)
	SSEConnectionOpened()
}

func (StreamMetrics) ConnectionClosed()              { SSEConnectionClosed() }
func (StreamMetrics) Disconnect(reason string)       { SSEDisconnect(reason) }
func (StreamMetrics) ConnectionDuration(t time.Time) { SSEConnectionDuration(t) }

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
	// isStream is set by [MarkStream] on the request goroutine,
	// between the middleware's two halves.
	isStream bool
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

// FlushError is what [http.ResponseController.Flush] prefers over Flush.
func (w *statusRW) FlushError() error {
	if f, ok := w.ResponseWriter.(interface{ FlushError() error }); ok {
		return f.FlushError()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
		return nil
	}
	return http.ErrNotSupported
}

// Unwrap returns the writer this one wraps,
// which is what [http.ResponseController] walks.
func (w *statusRW) Unwrap() http.ResponseWriter { return w.ResponseWriter }

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

// LabelUnmatched is the path label of a request that matched no route.
// The raw path is whatever the client asked for: labelling with it opens one
// time series per distinct path, which any visitor can then multiply.
const LabelUnmatched = "<unmatched>"

// LabelOtherMethod is the method label of a method no standard one covers.
// net/http accepts any RFC 7230 token as a method.
const LabelOtherMethod = "<other>"

// routeLabel is the pattern a request matched, which is one of the registered
// routes and hence bounded. A route variable stays a variable: /user/{uid}
// carries the requests of every user. A request that matched no route is
// labelled [LabelUnmatched].
func routeLabel(r *http.Request) string {
	if p := r.Pattern; p != "" {
		return p
	}
	return LabelUnmatched
}

// methodLabel folds anything but a standard method into [LabelOtherMethod].
func methodLabel(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodConnect,
		http.MethodOptions, http.MethodTrace:
		return method
	}
	return LabelOtherMethod
}

// MarkStream takes the request writing to w out of
// datapages_http_request_duration_seconds and datapages_http_in_flight_requests.
// A stream lives as long as the browser holds the page: observed as a request,
// an hour-long one lands above the top bucket and the gauge reads the number of
// connected browsers. Its count and lifetime are in
// datapages_http_sse_connections and datapages_sse_connection_duration_seconds.
//
// The mark rides on the writer rather than the request context, which would
// cost every request an allocation. It is a no-op unless w unwraps to the
// writer [Middleware] installed, the way [http.ResponseController] walks.
func MarkStream(w http.ResponseWriter) {
	for {
		if rw, ok := w.(*statusRW); ok {
			if !rw.isStream {
				rw.isStream = true
				mInFlightRequests.Dec()
			}
			return
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return
		}
		w = u.Unwrap()
	}
}

// Middleware measures every request. It must be the outermost middleware
// of the chain, otherwise it misses the work of the ones before it.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		rw := &statusRW{ResponseWriter: w, status: http.StatusOK}

		mInFlightRequests.Inc()
		defer func() {
			if !rw.isStream {
				mInFlightRequests.Dec()
			}
		}()

		next.ServeHTTP(rw, r)

		path, method := routeLabel(r), methodLabel(r.Method)
		mHTTPRequestsTotal.
			WithLabelValues(method, path, strconv.Itoa(rw.status)).Inc()
		if rw.isStream {
			return
		}
		mHTTPRequestDuration.
			WithLabelValues(method, path).Observe(time.Since(start).Seconds())
	})
}
