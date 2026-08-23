package main

import (
	"context"
	"errors"
	"flag"
	"net/http"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/counter/app/simple"
	"github.com/romshark/datapages/example/counter/app/simple/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	flag.Parse()

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

	err = s.ListenAndServe(context.Background(), *fHost)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
