package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/file-upload/app"
	"github.com/romshark/datapages/example/file-upload/app/datapagesgen"
	"github.com/romshark/datapages/example/file-upload/store"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	fDir := flag.String("dir", "uploads", "directory the uploaded files are kept in")
	flag.Parse()

	files, err := store.New(*fDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	a := app.NewApp(files)
	msgBroker := inmem.New(messaging.DefaultBrokerChanBuffer)
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, msgBroker,
		datapages.WithAssets(app.StaticFS, false),
		datapages.WithMiddleware(
			app.Downloads(a),
			app.ChunkDeadline(app.ChunkIdleTimeout),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "listening on http://%s\n", *fHost)
	err = s.ListenAndServe(context.Background(), *fHost)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
