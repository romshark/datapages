// Asserts that a path variable may carry any name the URL writer uses for a
// local of its own, and that a query tag needs to be no Go identifier.

package acceptance_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/hreflocals/app"
	"github.com/romshark/datapages/internal/acceptance/hreflocals/app/datapagesgen/action"
	"github.com/romshark/datapages/internal/acceptance/hreflocals/app/datapagesgen/href"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	return client.New(t, mustNewServer(t, &app.App{},
		inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// TestBuilderNameIsFree tests a path variable named after the URL writer's
// strings.Builder: the URL it builds addresses the route.
func TestBuilderNameIsFree(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	resp := c.Get(t, href.PageItem(true))
	require.Equal(t, http.StatusOK, resp.Status)
	require.Equal(t, "b=true", resp.Element(t, "echo"))
}

// TestEveryLocalNameIsFree tests the writer's other locals at once:
// the length, the counter, the flag and a query field's conversion variable.
func TestEveryLocalNameIsFree(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	url := href.PageMix(1, 2, "three", href.QueryPageMix{AnyQuery: "yes", Page: 4})
	resp := c.Get(t, url)

	require.Equal(t, http.StatusOK, resp.Status, url)
	require.Equal(t, "l=1 n=2 pageStr=three anyQuery=yes page=4",
		resp.Element(t, "echo"))
}

// TestNonIdentifierQueryTagHref tests query tags that are no Go identifier:
// the URL the href package builds carries them and addresses the route.
func TestNonIdentifierQueryTagHref(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	url := href.PageTags(href.QueryPageTags{PageSize: 25, Term: "go"})
	require.Equal(t, "/tags/?page-size=25&q.term=go", url)

	resp := c.Get(t, url)
	require.Equal(t, http.StatusOK, resp.Status, url)
	require.Equal(t, "page-size=25 q.term=go", resp.Element(t, "echo"))
}

// TestNonIdentifierQueryTagAction tests the same tag in an action expression:
// the value survives the URL the action package builds.
func TestNonIdentifierQueryTagAction(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	expr := action.POSTPageTagsSelect(action.QueryPOSTPageTagsSelect{PageSize: 7})

	// The expression a template carries: @post('<url>').
	const prefix, suffix = "@post('", "')"
	require.True(t, strings.HasPrefix(expr, prefix), expr)
	require.True(t, strings.HasSuffix(expr, suffix), expr)
	url := strings.TrimSuffix(strings.TrimPrefix(expr, prefix), suffix)
	require.Equal(t, "/tags/select/?page-size=7", url)

	require.Equal(t, http.StatusOK, c.Action(t, http.MethodPost, url, "").Status,
		"page-size did not reach the handler")
}

// TestParameterNamesAreFree tests a route wildcard named after a parameter the writer
// adds itself: the query struct of an href, the option variadic of an action helper.
func TestParameterNamesAreFree(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	url := href.PageParams("a", "b", href.QueryPageParams{Term: "x"})
	resp := c.Get(t, url)

	require.Equal(t, http.StatusOK, resp.Status, url)
	require.Equal(t, "query=a options=b t=x", resp.Element(t, "echo"))
}

// TestActionLocalNamesAreFree tests path variables named after
// every local the action writer declares for itself.
func TestActionLocalNamesAreFree(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	url := href.PageLocals("1", "2", "3", "4", "5")
	resp := c.Get(t, url)

	require.Equal(t, http.StatusOK, resp.Status, url)
	require.Equal(t, "b=1 l=2 n=3 bl=4 al=5", resp.Element(t, "echo"))

	expr := action.POSTPageLocalsSave("1", "2", "3", "4", "5")
	require.Equal(t, "@post('/locals/1/2/3/4/5/save/')", expr)
}

// TestActionLocalNamesAreFreeWithQuery tests the same names in an action that
// carries a path and a query, which is the writer that declares the most.
func TestActionLocalNamesAreFreeWithQuery(t *testing.T) {
	t.Parallel()
	c := newClient(t)

	expr := action.POSTPageMixStore(1, 2, "three",
		action.QueryPOSTPageMixStore{AnyQuery: "yes"})
	require.Equal(t, "@post('/mix/1/2/three/store/?anyQuery=yes')", expr)

	url := strings.TrimSuffix(strings.TrimPrefix(expr, "@post('"), "')")
	require.Equal(t, http.StatusOK,
		c.Action(t, http.MethodPost, url, "").Status,
		"the values did not survive the action URL")
}
