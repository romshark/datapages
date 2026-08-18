// Runs this case against a real NATS server.
//
// The broker built here is one half of the brokers table in events_test.go,
// so every test that drives the generated code runs twice: once on the in-memory
// implementation and once on the one an application deploys.
// The two match subjects with different code, and only one of them can say whether NATS
// accepts a subject the generator wrote.

package acceptance_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	natsctr "github.com/testcontainers/testcontainers-go/modules/nats"

	"github.com/romshark/datapages/internal/acceptance/events/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker"
	"github.com/romshark/datapages/modules/msgbroker/natscore"
)

// natsURL addresses the server every test in this file publishes through.
var natsURL string

func TestMain(m *testing.M) { os.Exit(runSuite(m)) }

func runSuite(m *testing.M) int {
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

// newNATSBroker connects to the server this package starts.
func newNATSBroker(t *testing.T) msgbroker.MessageBroker {
	t.Helper()

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

	return natscore.New(conn, natscore.Config{})
}

// TestNATSStreamSubjectsAreValid covers every subject the generator exports.
// A subject NATS refuses is a page that never receives,
// and the in-memory broker accepts anything a string can hold.
func TestNATSStreamSubjectsAreValid(t *testing.T) {
	conn, err := nats.Connect(natsURL)
	require.NoError(t, err, "connecting to NATS")
	t.Cleanup(conn.Close)

	subjects := datapagesgen.MessageBrokerStreamSubjects()
	require.NotEmpty(t, subjects, "the app exports no stream subjects")

	for _, subject := range subjects {
		sub, err := conn.SubscribeSync(subject)
		require.NoErrorf(t, err, "NATS refused the subscription subject %q", subject)
		require.NoError(t, sub.Unsubscribe())
	}
}
