package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/calculator/app"
	"github.com/romshark/datapages/example/calculator/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	flag.Parse()

	// The application dispatches no events: every update is the reply to the
	// action that caused it. NewServer still requires a broker.
	msgBroker := inmem.New(0)

	a := app.NewApp()
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, msgBroker, datapages.WithAssets(app.StaticFS, false))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: creating server: %v\n", err)
		os.Exit(1)
	}

	err = s.ListenAndServe(context.Background(), *fHost)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
