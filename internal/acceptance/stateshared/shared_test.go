// Drives a state type shared by two pages.

package acceptance_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/stateshared/app"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
)

func serve(t *testing.T) *datapagestest.Server {
	t.Helper()
	return datapagestest.New(t, mustNewServer(t,
		&app.App{}, inmem.New(messaging.DefaultBrokerChanBuffer)))
}

// bump sends the app action that increments the tab's counter.
func bump(t *testing.T, tab *datapagestest.Tab) *datapagestest.Response {
	t.Helper()
	return tab.Do(t, http.MethodPost, "/bump/", nil)
}

// note sends the action of PageIndex that sets the tab's note.
func note(t *testing.T, tab *datapagestest.Tab, text string) *datapagestest.Response {
	t.Helper()
	return tab.Do(t, http.MethodPost, "/note/", map[string]string{"note": text})
}

// state matches the patch of a tab's state, such as "{Counter:1 Note:hello}".
func state(text string) datapagestest.Match {
	return datapagestest.TextContains("state", text)
}

// TestSharedStateIsPerTab covers two tabs of one page bound to a shared state
// type. Sharing the type must not share the value.
func TestSharedStateIsPerTab(t *testing.T) {
	v := serve(t).Visitor(t)

	aTab := v.Load(t, "/")
	a := aTab.Connect(t, nil)
	bTab := v.Load(t, "/")
	b := bTab.Connect(t, nil)

	for range 2 {
		require.Equal(t, http.StatusOK, bump(t, aTab).Status, "bumping the first tab")
	}

	a.WaitFor(t, state("Counter:2"), "the first tab did not reach 2")

	// The second tab's own bump is the anchor. A patch for an event of the first tab
	// would arrive before it, and a value shared with the first tab would read Counter:3.
	require.Equal(t, http.StatusOK, bump(t, bTab).Status, "bumping the second tab")
	b.Absent(t, datapagestest.Element("state"), state("{Counter:1 Note:}"),
		"the second tab received the first tab's state")
}

// TestSharedStateIsPerPage covers two different pages bound to the same state
// type. Their tabs are still separate tabs.
func TestSharedStateIsPerPage(t *testing.T) {
	v := serve(t).Visitor(t)

	indexTab := v.Load(t, "/")
	index := indexTab.Connect(t, nil)
	otherTab := v.Load(t, "/other/")
	other := otherTab.Connect(t, nil)

	require.Equal(t, http.StatusOK, bump(t, otherTab).Status, "bumping the other page")

	other.WaitFor(t, state("Counter:1"))

	// The index tab's own note is the anchor. A patch for the event of the other page
	// would arrive before it, and a value shared with that page's tab would read
	// Counter:1.
	require.Equal(t, http.StatusOK, note(t, indexTab, "anchor").Status,
		"setting the note")
	index.Absent(t, datapagestest.Element("state"), state("{Counter:0 Note:anchor}"))
}

// TestPageActionAndAppActionShareTheState covers the two ways into one tab's
// state: an action of the page and an action of the app.
func TestPageActionAndAppActionShareTheState(t *testing.T) {
	tab := serve(t).Visitor(t).Load(t, "/")
	st := tab.Connect(t, nil)

	require.Equal(t, http.StatusOK, note(t, tab, "hello").Status, "setting the note")
	st.WaitFor(t, state("Note:hello"))

	// The app action acts on the value the page action wrote.
	require.Equal(t, http.StatusOK, bump(t, tab).Status, "bumping")
	st.WaitFor(t, state("{Counter:1 Note:hello}"))
}

// TestAppActionFromAStatelessPage covers a tab of a page that uses no state
// calling an app action that needs one. There is nothing to act on. The client
// is told to reconnect instead of being served another tab's value.
func TestAppActionFromAStatelessPage(t *testing.T) {
	tab := serve(t).Visitor(t).Load(t, "/plain/")

	require.Equal(t, http.StatusConflict, bump(t, tab).Status)
}
