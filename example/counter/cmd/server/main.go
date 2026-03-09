package main

import (
	"context"
	"errors"
	"flag"
	"net/http"

	"github.com/romshark/datapages/example/counter/app"
	"github.com/romshark/datapages/example/counter/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	flag.Parse()

	a := new(app.App)
	msgBroker := inmem.New(8)
	s := datapagesgen.NewServer(a, msgBroker)

	err := s.ListenAndServe(context.Background(), *fHost)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
