// Command server serves the app to a browser on 1 port, for datapages watch.
// Instead of opening windows, the page links to new tabs on the same port.
package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/desktop-multiwindow/app"
	"github.com/romshark/datapages/example/desktop-multiwindow/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](app.NewApp(nil), inmem.New(messaging.DefaultBrokerChanBuffer))
	if err != nil {
		panic(err)
	}

	err = s.ListenAndServe(ctx, *fHost)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
