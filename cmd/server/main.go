package main

import (
	"context"
	"datapages/app"
	"datapages/app/domain"
	"datapages/datapagesgen"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	"github.com/nats-io/nats.go"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "HTTP host address")
	fMsgBrokerMem := flag.Bool("msg-broker-inmem", false,
		"Forces in-memory message broker instead of NATS")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var opts []datapagesgen.ServerOption

	opts = withAccessLogger(opts)
	opts = withAuthJWT(opts)
	opts = withStaticFS(opts)
	opts = withMessageBroker(opts, *fMsgBrokerMem)

	repo := domain.NewRepository(mainCategories)

	s := datapagesgen.NewServer(app.NewApp(repo), opts...)

	go listenAndServe(s, *fHost)

	<-ctx.Done()
	slog.Info("shutting down server")
	if err := s.Shutdown(context.Background()); err != nil {
		slog.Error("shutting down server", slog.Any("err", err))
	}
	slog.Info("server shut down")
}

func withAccessLogger(opts []datapagesgen.ServerOption) []datapagesgen.ServerOption {
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
	return append(opts, o)
}

func withAuthJWT(opts []datapagesgen.ServerOption) []datapagesgen.ServerOption {
	o := datapagesgen.WithAuthJWTConfig(datapagesgen.AuthJWTConfig{
		Secret: []byte("myjwtsecret"),
	})
	return append(opts, o)
}

func withStaticFS(opts []datapagesgen.ServerOption) []datapagesgen.ServerOption {
	fsStatic, err := app.FSStatic()
	if err != nil {
		slog.Error("preparing static fs", slog.Any("err", err))
		os.Exit(1)
	}
	return append(opts,
		datapagesgen.WithStaticFS("/static/", fsStatic, app.FSStaticDev()))
}

func withMessageBroker(
	opts []datapagesgen.ServerOption,
	forceInmem bool,
) []datapagesgen.ServerOption {
	if forceInmem {
		slog.Info("forced in-memory message broker")
		return opts
	}

	// If NATS URL is set then enable NATS message broker.
	u := os.Getenv("NATS_URL")
	conn, err := nats.Connect(u)
	if err != nil {
		slog.Error("opening NATS connection", slog.Any("err", err))
		os.Exit(1)
	}
	opts = append(opts, datapagesgen.WithMessageBrokerNATS(
		conn, datapagesgen.MessageBrokerNATSConfig{
			StreamConfig: &nats.StreamConfig{
				Name:    "DATAPAGES_DEMO",
				Storage: nats.MemoryStorage,
			},
		},
	))
	slog.Info("using NATS message broker")
	return opts
}

func listenAndServe(s *datapagesgen.Server, host string) {
	var err error
	pathCert := os.Getenv("PATH_TLS_CERT")
	pathKey := os.Getenv("PATH_TLS_KEY")

	if pathCert == "" && pathKey == "" {
		slog.Info("listening HTTP", slog.String("host", host))
		err = s.ListenAndServe(host)
	} else {
		slog.Info("listening HTTPS",
			slog.String("host", host),
			slog.String("cert", pathCert),
			slog.String("key", pathKey))
		err = s.ListenAndServeTLS(host, pathCert, pathKey)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("listening", slog.Any("err", err))
	}
}
