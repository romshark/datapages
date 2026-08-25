// Runs this case against every broker datapages ships.
//
// The table lives in internal/acceptance/brokers, hence every test that reaches
// the generated code runs twice, once per implementation.
// What stays here is the one assertion only a real server can make.

package acceptance_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/brokers"
	"github.com/romshark/datapages/internal/acceptance/events/app/datapagesgen"
)

func TestMain(m *testing.M) { os.Exit(brokers.Main(m)) }

// TestNATSStreamSubjectsAreValid covers every subject the generator exports.
// A subject NATS refuses is a page that never receives,
// and the in-memory broker accepts anything a string can hold.
func TestNATSStreamSubjectsAreValid(t *testing.T) {
	t.Parallel()
	conn := brokers.Conn(t)

	subjects := datapagesgen.MessageBrokerStreamSubjects()
	require.NotEmpty(t, subjects, "the app exports no stream subjects")

	for _, subject := range subjects {
		sub, err := conn.SubscribeSync(subject)
		require.NoErrorf(t, err, "NATS refused the subscription subject %q", subject)
		require.NoError(t, sub.Unsubscribe())
	}
}
