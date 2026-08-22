package main

import (
	"context"
	"errors"
	"flag"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/counter/app/fancy"
	"github.com/romshark/datapages/example/counter/app/fancy/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	fHost := flag.String("host", "localhost:8081", "server host address")
	flag.Parse()

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

	err = s.ListenAndServe(context.Background(), *fHost)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
