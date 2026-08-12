// Drives the generated per-tab state of ./app.

package acceptance_test

import (
	"crypto/sha256"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/internal/acceptance/client"
	"github.com/romshark/datapages/internal/acceptance/stategeneric/app"
	"github.com/romshark/datapages/internal/acceptance/stategeneric/datapagesgen"
	"github.com/romshark/datapages/modules/msgbroker/inmem"
)

func newClient(t *testing.T) *client.Client {
	t.Helper()
	key := sha256.Sum256([]byte("acceptance"))
	return client.New(t, datapagesgen.NewServer(
		&app.App{}, inmem.New(8),
		datapagesgen.WithStateConfig(datapagesgen.StateConfig{HMACKey: key[:]}),
	))
}

// TestGenericStateIsPerTab covers two tabs of the same page. The generic base
// gives both the same handlers, and each tab must still hold its own value.
func TestGenericStateIsPerTab(t *testing.T) {
	c := newClient(t)

	a := c.OpenTab(t, "/count/", "")
	b := c.OpenTab(t, "/count/", "")

	require.NotEqual(t, a.InstanceID(), b.InstanceID(),
		"two page loads share one instance id")

	for _, tab := range []*client.Tab{a, a, b} {
		resp := tab.Act(t, http.MethodPost, "/count/bump/", "")
		require.Equal(t, http.StatusOK, resp.Status, "bumping")
	}

	require.True(t, a.Saw("{N:2}"), "the first tab did not reach 2")
	require.True(t, b.Saw("{N:1}"), "the second tab did not reach 1")
	require.True(t, b.Never("{N:2}"), "the second tab saw the first tab's count")
}

// TestGenericStateIsPerType covers the same generic base instantiated on two
// state types. Each page's handlers must be given its own type's value.
func TestGenericStateIsPerType(t *testing.T) {
	c := newClient(t)

	count := c.OpenTab(t, "/count/", "")
	label := c.OpenTab(t, "/label/", "")

	require.Equal(t, http.StatusOK,
		count.Act(t, http.MethodPost, "/count/bump/", "").Status, "bumping")
	require.Equal(t, http.StatusOK,
		label.Act(t, http.MethodPost, "/label/set/", `{"text":"hello"}`).Status,
		"setting the label")

	require.True(t, count.Saw("{N:1}"),
		"the counting page did not receive its own state")
	require.True(t, label.Saw("{Text:hello}"),
		"the labelling page did not receive its own state")
	require.True(t, count.Never("hello"),
		"a page received the state of a page of another type")
	require.True(t, label.Never("{N:"),
		"a page received the state of a page of another type")
}

// TestStateFromEmbedOnly covers a page whose stream, event handler and state
// type all come from an embedded generic page: it still gets a state value of
// its own.
func TestStateFromEmbedOnly(t *testing.T) {
	c := newClient(t)

	embed := c.OpenTab(t, "/embed-only/", "")
	label := c.OpenTab(t, "/label/", "")

	// The action belongs to another page bound to the same state type,
	// so it is the embedding page's own stream that must show its value.
	require.Equal(t, http.StatusOK,
		label.Act(t, http.MethodPost, "/label/set/", `{"text":"from label"}`).Status,
		"setting the label")
	require.True(t, label.Saw("{Text:from label}"), "the acting tab received nothing")
	require.True(t, embed.Never("from label"),
		"a tab of another page received the state")

	// The embedding page opened a stream of its own, which is what allocates
	// its state.
	resp := c.Get(t, "/")
	require.NotContains(t, resp.Body, "allocations=0",
		"the embed-only page opened no state")
}

// TestActionOfAnotherPage covers an action of one page called with the
// instance id of a tab bound to another. The state types differ and there is
// nothing to act on.
func TestActionOfAnotherPage(t *testing.T) {
	c := newClient(t)

	count := c.OpenTab(t, "/count/", "")

	resp := count.Act(t, http.MethodPost, "/label/set/", `{"text":"x"}`)
	require.NotEqual(t, http.StatusOK, resp.Status,
		"an action bound to another state type was served")
}

// TestStateIsAllocatedPerTab covers how many state values exist: one per tab.
func TestStateIsAllocatedPerTab(t *testing.T) {
	c := newClient(t)

	_ = c.OpenTab(t, "/count/", "")
	_ = c.OpenTab(t, "/count/", "")
	_ = c.OpenTab(t, "/label/", "")

	resp := c.Get(t, "/")
	require.Contains(t, resp.Body, "allocations=3",
		"three tabs did not open three state values")
}

// TestStateThroughNestedAbstracts covers a page whose state type arrives through
// two levels of generic abstract page: Mid[StateNested] embedding Base[S].
func TestStateThroughNestedAbstracts(t *testing.T) {
	c := newClient(t)

	a := c.OpenTab(t, "/nested/", "")
	b := c.OpenTab(t, "/nested/", "")

	for range 2 {
		require.Equal(t, http.StatusOK,
			a.Act(t, http.MethodPost, "/nested/bump/", "").Status, "bumping")
	}

	require.True(t, a.Saw("{N:2}"), "the tab did not reach 2 through the chain")
	require.True(t, b.Never("{N:2}"), "one tab sees the count of another")
}

// TestStateThroughPointerEmbed covers a page that embeds its abstract page by pointer.
func TestStateThroughPointerEmbed(t *testing.T) {
	c := newClient(t)

	tab := c.OpenTab(t, "/pointer/", "")

	require.Equal(t, http.StatusOK,
		tab.Act(t, http.MethodPost, "/pointer/bump/", "").Status, "bumping")

	require.True(t, tab.Saw("{N:1}"),
		"a page embedding its abstract by pointer got no state")
}
