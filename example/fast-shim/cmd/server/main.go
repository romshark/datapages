package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"

	"github.com/romshark/datapages/example/fast-shim/app"
	"github.com/romshark/datapages/example/fast-shim/datapagesgen"
	msgbrokerinmem "github.com/romshark/datapages/modules/msgbroker/inmem"
	"github.com/romshark/datapages/modules/offline"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	a := &app.App{}
	messageBroker := msgbrokerinmem.New(8)

	s := datapagesgen.NewServer(
		a, messageBroker,
		// Serves the worker that paints cached shims and morphs in the live page.
		// No PageOffline here; the worker keeps its own fallback.
		datapagesgen.WithMiddleware(offline.Middleware("", offline.Config{
			WorkerVersion: 3, // bump on worker changes to drop the old cache
		})),
	)

	slog.Info("listening", slog.String("addr", *fHost))
	if err := s.ListenAndServe(ctx, *fHost); err != nil &&
		!errors.Is(err, http.ErrServerClosed) {
		slog.Error("serving", slog.Any("err", err))
		os.Exit(1)
	}
}
