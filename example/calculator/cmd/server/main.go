package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/nats-io/nats.go"

	"github.com/romshark/datapages/example/calculator/app"
	"github.com/romshark/datapages/example/calculator/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker/natsjs"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	flag.Parse()

	hmacSecret := os.Getenv("HMAC_SECRET_KEY")
	if hmacSecret == "" {
		fmt.Fprintln(os.Stderr, "error: HMAC_SECRET_KEY env var is required")
		os.Exit(1)
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}
	nc, err := nats.Connect(natsURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: connecting to NATS: %v\n", err)
		os.Exit(1)
	}
	defer nc.Close()

	msgBroker, err := natsjs.New(nc, natsjs.Config{
		StreamConfig: &nats.StreamConfig{Name: "calculator"},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: initializing NATS broker: %v\n", err)
		os.Exit(1)
	}

	a := app.NewApp(sha256.Sum256([]byte(hmacSecret)))
	s := datapagesgen.NewServer(a, msgBroker,
		datapagesgen.WithAssets(app.StaticFS),
	)

	err = s.ListenAndServe(context.Background(), *fHost)
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}
