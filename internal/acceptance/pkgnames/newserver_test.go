package acceptance_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/pkgnames/app"
	"github.com/romshark/datapages/internal/acceptance/pkgnames/app/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/pkgnames/stream"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/sessions"
)

// mustNewServer builds the server the way datapages.NewServer does and fails the
// test on a configuration error, which no test can carry on from.
//
// The session data type argument is stream.SessionData, from the package named
// after runtime/stream. Naming it here is what a user writes, and the
// generated Init has to agree with it.
func mustNewServer(
	t *testing.T, a *app.App, broker messaging.Broker,
	sess sessions.Manager[stream.SessionData],
	opts ...datapages.ServerOption,
) datapages.Server {
	t.Helper()
	s, err := datapages.NewServer[
		app.App,
		stream.SessionData,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](
		a, broker,
		append([]datapages.ServerOption{
			datapages.WithSessionManager(sess),
		}, opts...)...,
	)
	require.NoError(t, err)
	return s
}
