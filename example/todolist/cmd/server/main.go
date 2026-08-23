package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/example/todolist/app"
	"github.com/romshark/datapages/example/todolist/app/datapagesgen"
	"github.com/romshark/datapages/example/todolist/list"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func main() {
	fHost := flag.String("host", "localhost:8080", "server host address")
	flag.Parse()

	hmacSecret := os.Getenv("HMAC_SECRET_KEY")
	if hmacSecret == "" {
		// This is fine for demo purposes.
		hmacSecret = "dev-secret-do-not-use-in-production"
	}

	l := new(list.List)
	now := time.Now()
	for _, s := range []struct {
		title, desc string
		done        bool
		due         time.Duration
	}{
		{"Buy groceries", "Milk, eggs, bread, and butter", false, 24 * time.Hour},
		{"Write report", "Q1 financial summary", false, 48 * time.Hour},
		{"Call dentist", "Schedule annual checkup", true, -24 * time.Hour},
		{"Read Go book", "Finish chapter on concurrency", false, 72 * time.Hour},
		{"Fix bike", "Replace brake pads", false, 168 * time.Hour},
	} {
		l.AddItem(s.title, s.desc, now.Add(s.due))
	}

	a := app.NewApp(sha256.Sum256([]byte(hmacSecret)), l)
	msgBroker := inmem.New(messaging.DefaultBrokerChanBuffer)
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](a, msgBroker, datapages.WithAssets(app.StaticFS))
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
