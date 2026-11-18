package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/pagesentinels/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func serve(t *testing.T) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestPageGETSentinelStatus tests page error statuses when the app has no
// error-returning action or custom error handler.
func TestPageGETSentinelStatus(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	for name, tc := range map[string]struct {
		path   string
		status int
	}{
		"bad request":    {"/bad-request/", http.StatusBadRequest},
		"forbidden":      {"/denied/", http.StatusForbidden},
		"not found":      {"/gone/", http.StatusNotFound},
		"wrapped":        {"/conflict/", http.StatusConflict},
		"without a code": {"/plain/", http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := v.Get(t, tc.path)

			require.Equal(t, tc.status, resp.Status)
			require.Equal(t, http.StatusText(tc.status)+"\n", resp.Body)
			require.NotContains(t, resp.Body, "no such item")
		})
	}
}

// TestPageIndexRenders tests the successful GET alongside the error pages.
func TestPageIndexRenders(t *testing.T) {
	t.Parallel()
	v := serve(t).Visitor(t)

	resp := v.Get(t, "/")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, resp.Body, `<pre id="echo">index</pre>`)
}
