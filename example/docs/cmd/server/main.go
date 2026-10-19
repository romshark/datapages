package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/docs/app"
	"github.com/romshark/datapages/example/docs/app/datapagesgen"
	"github.com/romshark/datapages/example/docs/app/datapagesgen/assets"
	"github.com/romshark/datapages/example/docs/images"
	"github.com/romshark/datapages/example/docs/store"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	fDir := flag.String("dir", "documents", "directory the documents are stored in")
	flag.Parse()

	if err := run(*fHost, *fDir); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(host, dir string) (err error) {
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	docs, err := store.Open(dir)
	if err != nil {
		return err
	}
	// ListenAndServe returns after the last handler and StreamClose hook.
	// Nothing writes to the store after that.
	defer func() { err = errors.Join(err, docs.Close()) }()
	imgs, err := images.Open(filepath.Join(dir, "images"))
	if err != nil {
		return err
	}

	a := app.NewApp(docs, imgs)
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithAssets(app.StaticFS, false),
		datapages.WithBodySizeLimit(app.BodySizeLimit),
		// The CDN is unreachable offline.
		datapages.WithDatastarJS(assets.Path("datastar-1.0.4.js")),
		datapagesgen.WithOffline(app.OfflineConfig()))
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "listening on http://%s, documents in %s\n", host, dir)
	err = s.ListenAndServe(ctx, host)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
