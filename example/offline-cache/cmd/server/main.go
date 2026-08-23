package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/offline-cache/app"
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen"
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen/assets"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// The entire demo runs on in-memory infrastructure — no external services
	// (NATS, databases) are required. All data is lost when the process exits.
	repo := NewRepository()
	a := app.NewApp(repo)

	messageBroker := inmem.New(messaging.DefaultBrokerChanBuffer)
	sessionManager := sessinmem.New[struct{}](sessions.DefaultTokenGenerator{
		Length: sessions.DefaultTokenLen,
	})

	s, err := datapages.NewServer[
		app.App,
		struct{},
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](
		a, messageBroker,
		datapages.WithSessionManager[struct{}](sessionManager),
		datapages.WithSessions(datapages.SessionsConfig{}),
		datapages.WithAssets(app.StaticFS),
		// Self-host Datastar so the app also works offline.
		datapages.WithDatastarJS(assets.Path("datastar.js")),
		// Service-worker-based offline support: serves the worker and injects
		// its registration into every page. The PageOffline route is supplied
		// by the generated option.
		datapagesgen.WithOffline(app.OfflineConfig()),
	)
	if err != nil {
		slog.Error("creating server", slog.Any("err", err))
		os.Exit(1)
	}

	slog.Info("listening", slog.String("addr", *fHost))
	if err := s.ListenAndServe(ctx, *fHost); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		slog.Error("serving", slog.Any("err", err))
		os.Exit(1)
	}
}
