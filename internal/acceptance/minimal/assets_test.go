package acceptance_test

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/internal/acceptance/minimal/app"
	"github.com/romshark/datapages/internal/acceptance/minimal/app/datapagesgen"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

// TestAssetsRefused tests datapages.WithAssets against an app package that
// declares no assets. The option has to say so instead of quietly serving
// nothing, which is what a missing stylesheet would look like at runtime.
func TestAssetsRefused(t *testing.T) {
	t.Parallel()
	s, err := datapages.NewServer[
		app.App,
		datapages.DisableSessions,
		datapages.DisablePrometheus,
		datapagesgen.Server,
	](&app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer),
		datapages.WithAssets(embed.FS{}))
	require.Nil(t, s)
	require.ErrorContains(t, err, "the app package declares no assets")
}
