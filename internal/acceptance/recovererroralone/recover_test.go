// Asserts that RecoverError is called for an app that defines it.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/recovererroralone/app"
	"github.com/romshark/datapages/internal/acceptance/recovererroralone/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, datapagesgen.NewServer(&app.App{}, inmem.New(8)))
}

// TestRecoverErrorIsCalled covers a failed action in an app that defines
// RecoverError and supplies no PageError500.
func TestRecoverErrorIsCalled(t *testing.T) {
	c := newClient(t)

	resp := c.Action(t, http.MethodPost, "/fail/", "")

	require.Contains(t, resp.Body, `<div id="toast">something went wrong</div>`,
		"RecoverError did not answer the failed request")
	require.NotContains(t, resp.Body, "the action failed",
		"the error message reached the client")
}

// TestPageLoadWithoutAn500Page covers a failed page load in the same app:
// with no page to render, the built-in response carries the status and
// nothing about the error.
func TestPageLoadWithoutAn500Page(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, "/fail/")
	require.NotEqual(t, http.StatusOK, resp.Status)
	require.NotContains(t, resp.Body, "the action failed")
}
