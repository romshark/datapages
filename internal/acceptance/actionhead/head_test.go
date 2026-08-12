// Asserts that the head an action returns reaches the response it renders.

package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/actionhead/app"
	"github.com/romshark/datapages/internal/acceptance/actionhead/datapagesgen"
	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, datapagesgen.NewServer(&app.App{}, inmem.New(8)))
}

// TestActionHeadIsRendered covers an action that returns a head of its own:
// it reaches the document, next to the app-wide head and before the body.
func TestActionHeadIsRendered(t *testing.T) {
	c := newClient(t)

	resp := c.Action(t, http.MethodPost, "/render/", "")
	require.Equal(t, http.StatusOK, resp.Status)

	require.Contains(t, resp.Body, `<meta name="from" content="action">`,
		"the head the action returned is not in the response")
	require.Contains(t, resp.Body, "body", "the body the action returned is missing")

	// The app-wide head is rendered too. One does not replace the other.
	require.Contains(t, resp.Body, "<title>global</title>",
		"the app-wide head is missing from a response that carries its own")

	// Both sit in the head, before the body opens.
	require.Less(t, strings.Index(resp.Body, `content="action"`),
		strings.Index(resp.Body, "<body"),
		"the action's head was written after the body opened")
}

// TestPageLoadCarriesTheGlobalHeadOnly covers a plain page load of the same app,
// which has no head of its own to add.
func TestPageLoadCarriesTheGlobalHeadOnly(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, "/")
	require.Equal(t, http.StatusOK, resp.Status)
	require.Contains(t, resp.Body, "<title>global</title>")
	require.NotContains(t, resp.Body, `content="action"`)
}
