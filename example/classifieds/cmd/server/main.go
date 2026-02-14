package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"

	csrfhmac "github.com/romshark/datapages/modules/csrf/hmac"
	"github.com/romshark/datapages/modules/msgbroker/natsjs"
	"github.com/romshark/datapages/modules/sessmanager/natskv"
	"github.com/romshark/datapages/modules/sesstokgen"

	"github.com/romshark/datapages/example/classifieds/app"
	"github.com/romshark/datapages/example/classifieds/datapagesgen"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	DefaultAppAddr     = "localhost"
	DefaultAppPort     = "8080"
	DefaultMetricsAddr = "localhost"
	DefaultMetricsPort = "9090"
)

func main() {
	host, port := DefaultAppAddr, DefaultAppPort
	if d := os.Getenv("HOST"); d != "" {
		host = d
	}
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var opts []datapagesgen.ServerOption

	withAccessLogger(&opts)
	withAuth(&opts)
	withCSRFProtection(&opts)
	withStaticFS(&opts)

	messageBroker, sessionManager := connectNATS()

	repo := NewRepository()
	a := app.NewApp(sessionManager, repo)
	initMetrics(&a.Metrics, &opts)

	s := datapagesgen.NewServer(a, messageBroker, sessionManager, opts...)
	listenAndServe(ctx, s, net.JoinHostPort(host, port))
}

func withAccessLogger(opts *[]datapagesgen.ServerOption) {
	loggerAccess := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	o := datapagesgen.WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loggerAccess.Info("access",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path))
			next.ServeHTTP(w, r)
		})
	})
	*opts = append(*opts, o)
}

func withStaticFS(opts *[]datapagesgen.ServerOption) {
	fsStatic, err := app.FSStatic()
	if err != nil {
		slog.Error("preparing static fs", slog.Any("err", err))
		os.Exit(1)
	}
	*opts = append(*opts,
		datapagesgen.WithStaticFS("/static/", fsStatic, app.FSStaticDev()))
}

func withAuth(opts *[]datapagesgen.ServerOption) {
	*opts = append(*opts, datapagesgen.WithAuth(datapagesgen.AuthConfig{}))
}

func withCSRFProtection(opts *[]datapagesgen.ServerOption) {
	tm, err := csrfhmac.New([]byte(os.Getenv("CSRF_SECRET")))
	if err != nil {
		slog.Error("initializing CSRF token manager", slog.Any("err", err))
		os.Exit(1)
	}
	*opts = append(*opts, datapagesgen.WithCSRFProtection(datapagesgen.CSRFConfig{
		TokenManager:   tm,
		DevBypassToken: os.Getenv("CSRF_DEV_BYPASS"),
	}))
}

func connectNATS() (
	messageBroker *natsjs.MessageBroker,
	sessionManager *natskv.SessionManager[app.Session],
) {
	// If NATS URL is set then enable NATS message broker.
	u := os.Getenv("NATS_URL")
	if u == "" {
		slog.Error("NATS_URL not set")
		os.Exit(2)
	}

	sessionEncryptionKey := os.Getenv("SESSION_ENCRYPTION_KEY")
	if sessionEncryptionKey == "" {
		slog.Error("SESSION_ENCRYPTION_KEY not set")
		os.Exit(2)
	}

	conn, err := nats.Connect(u)
	if err != nil {
		slog.Error("opening NATS connection", slog.Any("err", err))
		os.Exit(1)
	}
	slog.Info("using NATS message broker")

	sessionManager, err = natskv.New[app.Session](
		conn,
		sesstokgen.Generator{
			Length: sesstokgen.DefaultLength,
		},
		natskv.Config{
			EncryptionKey: []byte(sessionEncryptionKey),
		},
	)
	if err != nil {
		slog.Error("initializing NATS KV session manager", slog.Any("err", err))
		os.Exit(1)
	}
	slog.Info("using NATS KV session manager")

	messageBroker, err = natsjs.New(conn, natsjs.Config{
		StreamConfig: &nats.StreamConfig{
			Name:    "DATAPAGES_DEMO",
			Storage: nats.MemoryStorage,
		},
	})
	if err != nil {
		slog.Error("initializing NATS message broker", slog.Any("err", err))
		os.Exit(1)
	}

	return messageBroker, sessionManager
}

func initMetrics(m *app.Metrics, opts *[]datapagesgen.ServerOption) {
	m.LoginSubmissions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "app",
			Subsystem: "auth",
			Name:      "login_submissions_total",
			Help:      "Number of login submissions",
		},
		[]string{"result"},
	)
	m.ChatMessagesSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "app",
			Subsystem: "messaging",
			Name:      "chat_messages_sent_total",
			Help: "Total number of chat message " +
				"send attempts",
		},
		[]string{"result"},
	)

	host := os.Getenv("HOST_METRICS")
	port := os.Getenv("PORT_METRICS")
	if host == "" {
		host = DefaultMetricsAddr
	}
	if port == "" {
		port = DefaultMetricsPort
	}

	addr := net.JoinHostPort(host, port)
	*opts = append(*opts,
		datapagesgen.WithPrometheus(datapagesgen.PrometheusConfig{
			Host: addr,
			Collectors: []prometheus.Collector{
				m.LoginSubmissions,
				m.ChatMessagesSent,
			},
		}),
	)
}

func listenAndServe(ctx context.Context, s *datapagesgen.Server, host string) {
	var err error
	pathCert := os.Getenv("PATH_TLS_CERT")
	pathKey := os.Getenv("PATH_TLS_KEY")

	if pathCert == "" && pathKey == "" {
		err = s.ListenAndServe(ctx, host)
	} else {
		err = s.ListenAndServeTLS(ctx, host, pathCert, pathKey)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("listening", slog.Any("err", err))
	}
}
