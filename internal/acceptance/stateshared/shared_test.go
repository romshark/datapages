// Drives a state type shared by two pages.

package acceptance_test

import (
	"crypto/sha256"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/stateshared/app"
	"github.com/romshark/datapages/internal/acceptance/stateshared/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	key := sha256.Sum256([]byte("acceptance"))
	return client.New(t, datapagesgen.NewServer(
		&app.App{}, inmem.New(8),
		datapagesgen.WithStateConfig(datapagesgen.StateConfig{HMACKey: key[:]})))
}

// TestSharedStateIsPerTab covers two tabs of one page bound to a shared state
// type. Sharing the type must not share the value.
func TestSharedStateIsPerTab(t *testing.T) {
	c := newClient(t)

	a := c.OpenTab(t, "/", "")
	b := c.OpenTab(t, "/", "")

	for range 2 {
		require.Equal(t, http.StatusOK,
			a.Act(t, http.MethodPost, "/bump/", "").Status, "bumping the first tab")
	}

	require.True(t, a.Saw("Counter:2"), "the first tab did not reach 2")
	require.True(t, b.Never("Counter:"),
		"the second tab received the first tab's state")
}

// TestSharedStateIsPerPage covers two different pages bound to the same state
// type. Their tabs are still separate tabs.
func TestSharedStateIsPerPage(t *testing.T) {
	c := newClient(t)

	index := c.OpenTab(t, "/", "")
	other := c.OpenTab(t, "/other/", "")

	require.Equal(t, http.StatusOK,
		other.Act(t, http.MethodPost, "/bump/", "").Status, "bumping the other page")

	require.True(t, other.Saw("Counter:1"), "the page that acted received nothing")
	require.True(t, index.Never("Counter:1"),
		"a tab of another page received the state")
}

// TestPageActionAndAppActionShareTheState covers the two ways into one tab's
// state: an action of the page and an action of the app.
func TestPageActionAndAppActionShareTheState(t *testing.T) {
	c := newClient(t)

	tab := c.OpenTab(t, "/", "")

	require.Equal(t, http.StatusOK,
		tab.Act(t, http.MethodPost, "/note/", `{"note":"hello"}`).Status,
		"setting the note")
	require.True(t, tab.Saw("Note:hello"), "the page action did not write the state")

	require.Equal(t, http.StatusOK,
		tab.Act(t, http.MethodPost, "/bump/", "").Status, "bumping")
	require.True(t, tab.Saw("{Counter:1 Note:hello}"),
		"the app action did not act on the same value the page action wrote")
}

// TestAppActionFromAStatelessPage covers a tab of a page that uses no state
// calling an app action that needs one. There is nothing to act on. The client
// is told to reconnect instead of being served another tab's value.
func TestAppActionFromAStatelessPage(t *testing.T) {
	c := newClient(t)

	page := c.Get(t, "/plain/")

	req := c.Request(t, http.MethodPost, "/bump/", "")
	if id := page.Instance(); id != "" {
		req.Header.Set("Datapages-Instance", id)
	}
	require.Equal(t, http.StatusConflict, c.Do(t, req).Status)
}
