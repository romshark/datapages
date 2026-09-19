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
	"github.com/romshark/datapages/example/fast-shim/app"
	"github.com/romshark/datapages/example/fast-shim/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/offline"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	a := &app.App{}
	messageBroker := inmem.New(messaging.DefaultBrokerChanBuffer)

	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](
		a, messageBroker,
		// Serves the worker that paints cached shims and morphs in the live page.
		// No PageOffline here; the worker keeps its own fallback.
		datapages.WithMiddleware(offline.Middleware("", offline.Config{
			WorkerVersion: 3, // bump on worker changes to drop the old cache
		})),
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
