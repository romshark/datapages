package main

import (
	"context"
	"datapages/app"
	"datapages/datapagesgen"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"

	"github.com/nats-io/nats.go"
)

func main() {
	host, port := "localhost", "8080"
	if d := os.Getenv("HOST"); d != "" {
		host = d
	}
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	fMsgBrokerMem := flag.Bool("msg-broker-inmem", false,
		"Forces in-memory message broker instead of NATS")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var opts []datapagesgen.ServerOption

	withAccessLogger(&opts)
	withAuthJWT(&opts)
	withCSRFProtection(&opts)
	withStaticFS(&opts)
	withMessageBroker(&opts, *fMsgBrokerMem)
	withPrometheus(&opts)

	repo := NewRepository()

	s := datapagesgen.NewServer(app.NewApp(repo), opts...)
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

func withAuthJWT(opts *[]datapagesgen.ServerOption) {
	o := datapagesgen.WithAuthJWTConfig(datapagesgen.AuthJWTConfig{
		Secret: []byte(os.Getenv("JWT_SECRET")),
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

func withCSRFProtection(opts *[]datapagesgen.ServerOption) {
	*opts = append(*opts, datapagesgen.WithCSRFProtection(datapagesgen.CSRFConfig{
		Secret: []byte(os.Getenv("CSRF_SECRET")),
	}))
}

func withMessageBroker(
	opts *[]datapagesgen.ServerOption,
	forceInmem bool,
) {
	if forceInmem {
		slog.Info("forced in-memory message broker")
		return
	}
	// If NATS URL is set then enable NATS message broker.
	u := os.Getenv("NATS_URL")
	if u == "" {
		slog.Warn("NATS_URL not set; using in-memory message broker")
		return
	}
	conn, err := nats.Connect(u)
	if err != nil {
		slog.Error("opening NATS connection", slog.Any("err", err))
		os.Exit(1)
	}
	*opts = append(*opts, datapagesgen.WithMessageBrokerNATS(
		conn, datapagesgen.MessageBrokerNATSConfig{
			StreamConfig: &nats.StreamConfig{
				Name:    "DATAPAGES_DEMO",
				Storage: nats.MemoryStorage,
			},
		},
	))
	slog.Info("using NATS message broker")
}

func withPrometheus(opts *[]datapagesgen.ServerOption) {
	host := os.Getenv("HOST_METRICS")
	port := os.Getenv("PORT_METRICS")

	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "9090"
	}

	addr := net.JoinHostPort(host, port)
	*opts = append(*opts, datapagesgen.WithPrometheus(datapagesgen.PrometheusConfig{
		Host: addr,
		// Registerer/Gatherer left nil => defaults
	}))
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
