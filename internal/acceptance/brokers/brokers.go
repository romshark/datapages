// Package brokers runs an acceptance case against every message broker datapages ships.
//
// A case states what it expects of event delivery once, and both
// implementations have to meet it. They are unrelated code: inmem matches
// subjects with its own matcher in this repository, natscore leaves matching to
// a NATS server. An expectation asserted against one of them is an expectation
// an application cannot rely on.
package brokers

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	natsctr "github.com/testcontainers/testcontainers-go/modules/nats"

	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/messaging/natscore"
)

// ChanBuffer is the subscription buffer every broker built here is given.
const ChanBuffer = 8

// natsURL addresses the server Main starts, empty until it has.
var natsURL string

// Main starts the NATS server the "nats" broker connects to, runs the case,
// and takes the server down again. A case that calls Each needs it:
//
//	func TestMain(m *testing.M) { os.Exit(brokers.Main(m)) }
func Main(m *testing.M) int {
	ctx := context.Background()

	ctr, err := natsctr.Run(ctx, "nats:latest")
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting NATS container: %v\n", err)
		return 1
	}
	defer func() { _ = ctr.Terminate(ctx) }()

	natsURL, err = ctr.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reading NATS connection string: %v\n", err)
		return 1
	}
	return m.Run()
}

// Each runs body once per broker, each with a broker and a subtest of its own.
func Each(t *testing.T, body func(t *testing.T, broker messaging.Broker)) {
	t.Helper()
	build := map[string]func(t *testing.T) messaging.Broker{
		"inmem": func(*testing.T) messaging.Broker {
			return inmem.New(ChanBuffer)
		},
		"nats": NATS,
	}
	for name, newBroker := range build {
		t.Run(name, func(t *testing.T) { body(t, newBroker(t)) })
	}
}

// NATS builds a broker on the server Main started.
func NATS(t *testing.T) messaging.Broker {
	t.Helper()
	return natscore.New(Conn(t), natscore.Config{ChanBuffer: ChanBuffer})
}

// Conn connects to the server Main started.
func Conn(t *testing.T) *nats.Conn {
	t.Helper()
	require.NotEmpty(t, natsURL,
		"no NATS server: the case must run brokers.Main from its TestMain")

	// The testcontainers NATS module only waits for the port to be open,
	// not for the server to be fully initialized.
	// Use nats.RetryOnFailedConnect so the client keeps retrying until NATS is ready.
	conn, err := nats.Connect(
		natsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(50),
		nats.ReconnectWait(200*time.Millisecond),
	)
	require.NoError(t, err, "connecting to NATS")
	require.Eventually(t, conn.IsConnected, 10*time.Second, 100*time.Millisecond)
	t.Cleanup(conn.Close)
	return conn
}
