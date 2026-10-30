package app_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen/action"
	"github.com/romshark/datapages/example/offline-cache/app/datapagesgen/href"
	"github.com/romshark/datapages/example/offline-cache/app/domain"
)

// TestPurchaseSoldOut tests that the visitor who loses the race for the last
// ticket is told so and gets the purchase button back.
// The button stays disabled while $_buying is true.
func TestPurchaseSoldOut(t *testing.T) {
	repo := domain.NewRepository()
	const price, available = 10, 1
	_, err := repo.AddShow(t.Context(), "Last seat", "", "Concert", "Hall",
		"Berlin", "", time.Now().Add(24*time.Hour), price, available)
	require.NoError(t, err, "adding show")
	shows, err := repo.SearchShows(t.Context(), "")
	require.NoError(t, err, "listing shows")
	require.Len(t, shows, 1)
	slug := shows[0].Slug
	confirm := action.PagePurchase.Confirm.POST(slug)
	newUser(t, repo, "winner")
	newUser(t, repo, "loser")
	srv := newServer(t, repo)
	winner, loser := signIn(t, srv, "winner"), signIn(t, srv, "loser")

	// Both open the purchase page while the last ticket is still available.
	winnerTab := winner.Load(t, href.PagePurchase(slug))
	loserTab := loser.Load(t, href.PagePurchase(slug))

	resp := winnerTab.Do(t, confirm, nil)
	require.Equal(t, http.StatusOK, resp.Status, "winner: %s", resp.Body)

	resp = loserTab.Do(t, confirm, nil).RequireStatus(t, http.StatusOK)
	resp.Event(t, soldOutToast)
	require.JSONEq(t, `{"_buying":false}`,
		resp.Event(t, datapagestest.Signal("_buying")).Data["signals"])
}

// soldOutToast matches the toast that RecoverError appends to #toaster for a
// sold-out show. The toast has no id.
var soldOutToast = datapagestest.Func("a toast appended to #toaster saying sold out",
	func(e datapagestest.Event) bool {
		return e.Type == datapagestest.PatchElements &&
			e.Data["selector"] == "#toaster" && e.Data["mode"] == "append" &&
			strings.Contains(datapagestest.Text(e.Root()), "Sold out.")
	})
