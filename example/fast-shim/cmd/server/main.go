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
		// Serve cached shims immediately and replace them with the live page.
		// The worker uses its own fallback because the application has no PageOffline.
		// Install it directly with [offline.WithServiceWorker].
		offline.WithServiceWorker("", offline.Config{
			// Increment after a Datapages upgrade or a change to an
			// [offline.Config] field embedded in the worker script.
			WorkerVersion: 3,
		}),
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
