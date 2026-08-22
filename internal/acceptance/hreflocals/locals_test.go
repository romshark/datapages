// Asserts that a path variable may carry any name the URL writer uses for a
// local of its own.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/hreflocals/app"
	"github.com/romshark/datapages/internal/acceptance/hreflocals/app/datapagesgen/href"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestBuilderNameIsFree covers a path variable named after the URL writer's
// strings.Builder: the URL it builds addresses the route.
func TestBuilderNameIsFree(t *testing.T) {
	c := newClient(t)

	resp := c.Get(t, href.PageItem(true))
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "b=true", resp.Element(t, "echo"))
}

// TestEveryLocalNameIsFree covers the writer's other locals at once:
// the length, the counter, the flag and a query field's conversion variable.
func TestEveryLocalNameIsFree(t *testing.T) {
	c := newClient(t)

	url := href.PageMix(1, 2, "three", href.QueryPageMix{AnyQuery: "yes", Page: 4})
	resp := c.Get(t, url)

	require.Equal(t, http.StatusOK, resp.Status, url)
	require.Equal(t, "l=1 n=2 pageStr=three anyQuery=yes page=4",
		resp.Element(t, "echo"))
}
