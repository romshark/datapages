package app_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages"
	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/example/classifieds/app"
	"github.com/romshark/datapages/example/classifieds/app/datapagesgen"
	"github.com/romshark/datapages/example/classifieds/app/datapagesgen/href"
	"github.com/romshark/datapages/example/classifieds/app/domain"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/messaging/inmem"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

// TestPageSettingsPOSTSave tests that a rename closes every session of the old
// name and no session of another user, and that a refused rename closes none.
// alice saves on her laptop while her phone is signed in too, with the settings
// page open. After a rename the laptop holds a session of the new name, which
// expires when the old one would have, and the phone's page leaves its closed
// session. A save that renames nobody sends the phone's page nothing.
func TestPageSettingsPOSTSave(t *testing.T) {
	for name, tc := range map[string]struct {
		username   string
		wantClosed bool   // alice's sessions
		wantUserID string // of the session issued, "" for none
		wantToast  string // text of the toast RecoverError reports the error in
	}{
		"rename":     {username: "alice2", wantClosed: true, wantUserID: "alice2"},
		"unchanged":  {username: "alice"},
		"name taken": {username: "bob", wantToast: "That user name is taken."},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			repo := domain.NewRepository(nil, domain.NewSeededSlugNonceGenerator(1, 2))
			for _, u := range []string{"alice", "bob"} {
				_, err := repo.NewUser(ctx, u, "", u+"@example.com", "pw-"+u)
				require.NoError(t, err)
			}

			store := sessinmem.New[struct{}](sessions.DefaultTokenGenerator{})
			broker := inmem.New(messaging.DefaultBrokerChanBuffer)
			srv := newServer(t, app.NewApp(store, repo), broker, store)

			expiresAt := time.Now().Add(time.Hour)
			signIn := func(userID string) string {
				token, err := store.CreateSession(ctx, sessions.Record[struct{}]{
					UserID: userID, IssuedAt: time.Now(), ExpiresAt: expiresAt,
				})
				require.NoError(t, err)
				return token
			}
			laptopToken, phoneToken := signIn("alice"), signIn("alice")
			bobToken := signIn("bob")
			isOpen := func(token string) bool {
				_, err := store.Session(ctx, token)
				if errors.Is(err, sessions.ErrSessionNotFound) {
					return false
				}
				require.NoError(t, err)
				return true
			}

			phone := srv.Visitor(t)
			phone.SetSessionToken(phoneToken)
			phoneTab := phone.Load(t, href.PageSettings())
			phoneStream := phoneTab.Connect(t, phoneTab.Page().PublicSignals(t))

			laptop := srv.Visitor(t)
			laptop.SetSessionToken(laptopToken)
			resp := laptop.Load(t, href.PageSettings()).Do(t, http.MethodPost,
				"/settings/save/", map[string]string{"username": tc.username})
			// RecoverError answers a failed action with 200 and the events it writes.
			require.Equal(t, http.StatusOK, resp.Status, resp.Body)

			require.Equal(t, !tc.wantClosed, isOpen(laptopToken))
			require.Equal(t, !tc.wantClosed, isOpen(phoneToken))
			require.True(t, isOpen(bobToken))

			cookie := laptop.Cookie(datapages.DefaultSessionCookieName)
			require.NotNil(t, cookie, "the laptop holds no session cookie")
			if tc.wantUserID == "" {
				require.Equal(t, laptopToken, cookie.Value, "the save issued a session")
			} else {
				require.NotEqual(t, laptopToken, cookie.Value,
					"the save issued no session")
				rec, err := store.Session(ctx, cookie.Value)
				require.NoError(t, err, "reading the issued session")
				require.Equal(t, tc.wantUserID, rec.UserID)
				require.Equal(t, expiresAt, rec.ExpiresAt)
			}

			if !tc.wantClosed {
				if tc.wantToast != "" {
					resp.Event(t, toast(tc.wantToast))
				} else {
					require.Empty(t, resp.Body,
						"the save answered with a redirect or a patch")
				}
				// The phone's page answers an EventMessagingRead with a patch of its
				// messages link. Published after the save, the event is the anchor that
				// shows the save sent the phone's page nothing.
				datapagestest.Publish(t, broker,
					datapagestest.Subject(datapagesgen.EvPrefixMessagingRead, "alice"),
					app.EventMessagingRead{Recipient: "alice"})
				phoneStream.Absent(t, anyEvent, messagesLink)
				return
			}
			require.Equal(t, href.PageSettings(), resp.Redirect(t))
			phoneStream.WaitFor(t, leaveSession,
				"the phone's page was not told to leave its closed session")
			phoneStream.WaitEnd(t, "the phone's stream outlived its session")
		})
	}
}

// newServer serves a with the NewServer type arguments of cmd/server, on the
// in-memory broker and session store. It leaves out the access log, the assets and
// the application metrics, which no request of the test needs.
func newServer(
	t *testing.T, a *app.App, broker messaging.Broker,
	store *sessinmem.SessionManager[struct{}],
) *datapagestest.Server {
	t.Helper()
	s, err := datapages.NewServer[
		app.App,
		struct{},
		datapages.EnablePrometheus,
		datapagesgen.Server,
	](
		a, broker,
		datapages.WithSessionManager(store),
		// datapages gen requires every NewServer call of the app to name the
		// EnablePrometheus of cmd/server, and EnablePrometheus requires
		// WithPrometheus. Only ListenAndServe starts the metrics server.
		datapages.WithPrometheus(datapages.PrometheusConfig{Host: "127.0.0.1:0"}),
	)
	require.NoError(t, err, "building server")
	return datapagestest.New(t, s)
}

// leaveSession matches what takes a settings page off its closed session: the
// redirect to the sign-in page that OnSessionClosed sends, or the script that
// reloads the page, which the server sends when it ends the stream of a closed
// session. Either can come first: the stream can see the session close before
// the event arrives, and the event then never reaches the page.
var leaveSession = datapagestest.Func(
	"the redirect to the sign-in page or the script that reloads the page",
	func(e datapagestest.Event) bool {
		if target, ok := e.Redirect(); ok {
			return target == href.PageLogin()
		}
		return datapagestest.Reload().Matches(e)
	})

var anyEvent = datapagestest.Func("an event", func(datapagestest.Event) bool {
	return true
})

// messagesLink matches the patch of the messages link that OnMessagingRead sends.
// A render of the whole page, which OnSessionClosed sends, contains the link too.
var messagesLink = datapagestest.Func("a patch of the messages link alone",
	func(e datapagestest.Event) bool {
		return datapagestest.Element("messages").Matches(e) &&
			!datapagestest.Element("page-settings").Matches(e)
	})

// toast matches a toast containing text that RecoverError appends to #toaster.
// A toast has no id.
func toast(text string) datapagestest.Match {
	what := fmt.Sprintf("a toast appended to #toaster containing %q", text)
	return datapagestest.Func(what,
		func(e datapagestest.Event) bool {
			return e.Type == datapagestest.PatchElements &&
				e.Data["selector"] == "#toaster" && e.Data["mode"] == "append" &&
				strings.Contains(datapagestest.Text(e.Root()), text)
		})
}
