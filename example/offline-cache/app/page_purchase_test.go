package app_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
	confirm := "/shows/" + shows[0].Slug + "/purchase/confirm/"
	newUser(t, repo, "winner")
	newUser(t, repo, "loser")
	srv := newServer(t, repo)
	winner, loser := signIn(t, srv, "winner"), signIn(t, srv, "loser")

	resp, body := winner.post(t, confirm, struct{}{})
	require.Equal(t, http.StatusOK, resp.StatusCode, "winner: %s", body)

	resp, body = loser.post(t, confirm, struct{}{})
	require.Equal(t, http.StatusOK, resp.StatusCode, "loser: %s", body)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	require.Contains(t, body, "Sold out.")
	require.Contains(t, body, `{"_buying":false}`)
}
