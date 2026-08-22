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

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/classifieds/app"
	"github.com/romshark/datapages/example/classifieds/app/datapagesgen"
	"github.com/romshark/datapages/example/classifieds/app/datapagesgen/assets"
	"github.com/romshark/datapages/modules/messaging/natscore"
	"github.com/romshark/datapages/modules/sessions"
	"github.com/romshark/datapages/modules/sessions/natskv"
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

	var opts []datapages.ServerOption

	if os.Getenv("DISABLE_ACCESS_LOG") == "" {
		withAccessLogger(&opts)
	}
	withSessions(&opts)
	withAssets(&opts)

	messageBroker, sessionManager := connectNATS()

	repo := NewRepository()
	a := app.NewApp(sessionManager, repo)
	initMetrics(&a.Metrics, &opts)

	opts = append(opts, datapages.WithSessionManager(sessionManager))

	s, err := datapages.NewServer[
		app.App,
		struct{},
		datapages.EnablePrometheus,
		datapagesgen.Server,
	](a, messageBroker, opts...)
	if err != nil {
		slog.Error("creating server", slog.Any("err", err))
		os.Exit(1)
	}
	listenAndServe(ctx, s, net.JoinHostPort(host, port))
}

func withAccessLogger(opts *[]datapages.ServerOption) {
	loggerAccess := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	o := datapages.WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loggerAccess.Info("access",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path))
			next.ServeHTTP(w, r)
		})
	})
	*opts = append(*opts, o)
}

func withAssets(opts *[]datapages.ServerOption) {
	*opts = append(*opts,
		datapages.WithAssets(app.StaticFS),
		datapages.WithDatastarJS(assets.Path("ds.min.js")))
}

func withSessions(opts *[]datapages.ServerOption) {
	*opts = append(*opts, datapages.WithSessions(datapages.SessionsConfig{}))
}

func connectNATS() (
	messageBroker *natscore.MessageBroker,
	sessionManager *natskv.SessionManager[struct{}],
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

	sessionManager, err = natskv.New[struct{}](
		conn,
		sessions.DefaultTokenGenerator{
			Length: sessions.DefaultTokenLen,
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

	messageBroker = natscore.New(conn, natscore.Config{})

	return messageBroker, sessionManager
}

func initMetrics(m *app.Metrics, opts *[]datapages.ServerOption) {
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
			Help:      "Total number of chat message send attempts",
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
	*opts = append(
		*opts,
		datapages.WithPrometheus(datapages.PrometheusConfig{
			Host: addr,
			Collectors: []prometheus.Collector{
				m.LoginSubmissions,
				m.ChatMessagesSent,
			},
		}),
	)
}

func listenAndServe(ctx context.Context, s datapages.Server, host string) {
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
