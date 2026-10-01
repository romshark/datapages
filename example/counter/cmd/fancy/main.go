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
	"github.com/romshark/datapages/example/counter/app/fancy"
	"github.com/romshark/datapages/example/counter/app/fancy/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	fHost := flag.String("host", "localhost:8081", "server host address")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	a := new(fancy.App)
	msgBroker := inmem.New(messaging.DefaultBrokerChanBuffer)
	s, err := datapages.NewServer[
		fancy.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, msgBroker)
	if err != nil {
		panic(err)
	}

	err = s.ListenAndServe(ctx, *fHost)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
