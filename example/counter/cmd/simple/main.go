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
	"github.com/romshark/datapages/example/counter/app/simple"
	"github.com/romshark/datapages/example/counter/app/simple/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	a := new(simple.App)
	msgBroker := inmem.New(messaging.DefaultBrokerChanBuffer)
	s, err := datapages.NewServer[
		simple.App,
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
