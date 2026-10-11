// Tests the generated session handling of ./app.

package acceptance_test

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/romshark/datapages/datapagestest"
	"github.com/romshark/datapages/internal/acceptance/brokers"
	"github.com/romshark/datapages/internal/acceptance/sessions/app"
	"github.com/romshark/datapages/modules/csrf"
	"github.com/romshark/datapages/modules/messaging"
	"github.com/romshark/datapages/modules/sessions"
	sessinmem "github.com/romshark/datapages/modules/sessions/inmem"
)

const cookieName = "sessiontoken"

// server is a datapagestest server of the app and the session store it was built with.
type server struct {
	*datapagestest.Server
	sessions *sessinmem.SessionManager[app.SessionData]
}

func TestMain(m *testing.M) { os.Exit(brokers.Main(m)) }

// An app that declares a Session type is CSRF protected without being given
// anything: the token is derived from the session token.
func serve(t *testing.T, broker messaging.Broker) server {
	t.Helper()
	store := sessinmem.New[app.SessionData](
		sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen},
	)
	return server{
		Server:   datapagestest.New(t, mustNewServer(t, &app.App{}, broker, store)),
		sessions: store,
	}
}

// signIn signs v in through the sign-in action and returns its response.
// A guest sends it without a CSRF token: an anonymous request needs none.
func signIn(
	t *testing.T, v *datapagestest.Visitor, user, nickname string,
) *datapagestest.Response {
	t.Helper()
	resp := v.Load(t, "/login/").Do(t, http.MethodPost, "/login/submit/",
		map[string]string{"user": user, "nickname": nickname})
	if resp.Status != http.StatusOK {
		t.Fatalf("signing in: status = %d\n%s", resp.Status, resp.Body)
	}
	return resp
}

// notify is the signals of the action that dispatches a notice to user.
func notify(user, text string) map[string]string {
	return map[string]string{"user": user, "text": text}
}

// roomUpdate is the signals of the action that dispatches a room update to user on
// the calc_id calcID.
func roomUpdate(user, calcID, data string) map[string]string {
	return map[string]string{"user": user, "room": "r1", "calc_id": calcID, "data": data}
}

// renameRequest builds the rename action of a signed-in v, with the CSRF token of
// the page it loads. A test may change the token before it sends the request.
func renameRequest(
	t *testing.T, v *datapagestest.Visitor, nickname string,
) *http.Request {
	t.Helper()
	req := v.Load(t, "/login/").NewRequest(t, http.MethodPost, "/login/rename/",
		strings.NewReader(`{"nickname":"`+nickname+`"}`))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// csrfToken is the token derived from sessionToken as a string,
// which is what a test sends where the browser would send the header.
func csrfToken(t *testing.T, sessionToken string) string {
	t.Helper()
	var b strings.Builder
	_, err := csrf.Tokens{}.WriteToken(&b, sessionToken)
	require.NoError(t, err, "writing the CSRF token")
	return b.String()
}

// setCookie returns the session cookie resp sets, with its attributes, or nil.
func setCookie(resp *datapagestest.Response) *http.Cookie {
	for _, ck := range resp.Cookies() {
		if ck.Name == cookieName {
			return ck
		}
	}
	return nil
}

// sessionToken is the token the visitor's session cookie carries,
// which is what the CSRF token is derived from.
func sessionToken(t *testing.T, v *datapagestest.Visitor) string {
	t.Helper()
	ck := v.Cookie(cookieName)
	if ck == nil {
		t.Fatal("the visitor holds no session cookie")
	}
	return ck.Value
}

// echoed is the text of the element the app echoes what a handler received into.
func echoed(t *testing.T, resp *datapagestest.Response) string {
	t.Helper()
	return datapagestest.Text(resp.ElementByID(t, "echo"))
}

// TestAnonymous tests a visitor with no session.
// Every handler that asks for one gets the zero value rather than an error.
func TestAnonymous(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		resp := v.Get(t, "/")
		if resp.Status != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.Status)
		}
		if got, want := echoed(t, resp), "anonymous"; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
		if !strings.Contains(resp.Body, "<title>anonymous</title>") {
			t.Errorf("the head did not see the empty session:\n%s", resp.Body)
		}
		if v.Cookie(cookieName) != nil {
			t.Error("a visitor with no session was given a session cookie")
		}
	})
}

// TestDocumentCacheControl tests the cache header of a document. One rendered
// for a session carries a CSRF token derived from it, which a shared cache would
// serve to the next visitor, whose actions then fail with 403: it is private.
// The 404 page, which has a status of its own to send, is private too.
// A guest's document carries no token and stays cacheable.
func TestDocumentCacheControl(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)

		guest := srv.Visitor(t).Get(t, "/")
		require.Empty(t, guest.Header.Get("Cache-Control"), "the page of a guest")

		v := srv.Visitor(t)
		signIn(t, v, "alice", "Alice")
		for path, wantStatus := range map[string]int{
			"/":         http.StatusOK,
			"/missing/": http.StatusNotFound,
		} {
			resp := v.Get(t, path)
			require.Equal(t, wantStatus, resp.Status, "%s\n%s", path, resp.Body)
			require.Equal(t, "private", resp.Header.Get("Cache-Control"), path)
		}
	})
}

// TestFileVary tests the Vary header of a file. One whose handler takes the
// session varies by cookie: a browser keys its cache by URL, and after a sign-in
// as another user it would revalidate the copy it stored for the first.
// One whose handler takes no session varies by nothing.
func TestFileVary(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)
		signIn(t, v, "alice", "Alice")

		for path, want := range map[string]struct {
			body string
			vary []string
		}{
			"/me/avatar": {"avatar of alice", []string{"Cookie"}},
			"/terms":     {"terms", nil},
		} {
			resp := v.Get(t, path)
			require.Equal(t, http.StatusOK, resp.Status, "%s\n%s", path, resp.Body)
			require.Equal(t, want.body, resp.Body, path)
			require.Equal(t, want.vary, resp.Header.Values("Vary"), path)
		}
	})
}

// TestCookieHeaderVariants tests the Cookie header shapes the generated
// reader must read the way net/http.Request.Cookie reads them.
// The expectation comes from that method, not from a hand written table.
func TestCookieHeaderVariants(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)
		alice := srv.Visitor(t)
		signIn(t, alice, "alice", "Al")
		token := sessionToken(t, alice)

		for name, header := range map[string]string{
			"plain":               cookieName + "=" + token,
			"among other pairs":   "theme=dark; " + cookieName + "=" + token + "; lang=en",
			"quoted value":        cookieName + `="` + token + `"`,
			"space before name":   " " + cookieName + "=" + token,
			"space after name":    cookieName + " =" + token,
			"invalid value byte":  cookieName + `=a"b`,
			"invalid then valid":  cookieName + `=a"b; ` + cookieName + "=" + token,
			"another cookie only": "theme=dark",
			"empty value":         cookieName + "=",
			"empty header":        "",
			"many other pairs": strings.Repeat("a=b; ", 200) +
				cookieName + "=" + token,
		} {
			t.Run(name, func(t *testing.T) {
				want := "anonymous"
				oracle := &http.Request{
					Header: http.Header{"Cookie": []string{header}},
				}
				if ck, err := oracle.Cookie(cookieName); err == nil &&
					ck.Value == token {
					want = "user=alice"
				}

				// A visitor whose jar is empty sends the header as it is written.
				v := srv.Visitor(t)
				req := v.NewRequest(t, http.MethodGet, "/", nil)
				if header != "" {
					req.Header.Set("Cookie", header)
				}
				require.Contains(t, v.Send(t, req).Body, want)
			})
		}
	})
}

// TestSignInAndOut tests the whole life of a session: an action creates it,
// later requests carry it, and an action ends it.
func TestSignInAndOut(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)
		v := srv.Visitor(t)

		ck := setCookie(signIn(t, v, "alice", "Al"))
		if ck == nil {
			t.Fatal("signing in set no session cookie")
		}
		if !ck.HttpOnly {
			t.Error("the session cookie is readable by scripts")
		}
		if ck.SameSite == http.SameSiteNoneMode {
			t.Error("the session cookie is sent on cross-site requests")
		}
		if v.Cookie(cookieName) == nil {
			t.Error("the session cookie was not kept by the client")
		}

		index := v.Load(t, "/")
		rec, err := srv.sessions.Session(t.Context(), sessionToken(t, v))
		require.NoError(t, err, "reading the session record")
		want := "user=alice nickname=Al issued=" +
			strconv.FormatInt(rec.IssuedAt.Unix(), 10)
		if got := echoed(t, index.Page()); got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
		if !strings.Contains(index.Page().Body, "<title>alice</title>") {
			t.Errorf("the head did not see the session:\n%s", index.Page().Body)
		}

		resp := index.Do(t, http.MethodPost, "/sign-out/", nil)
		if resp.Status != http.StatusOK {
			t.Fatalf("signing out: status = %d\n%s", resp.Status, resp.Body)
		}

		resp = v.Get(t, "/")
		if resp.Status != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.Status)
		}
		if got, want := echoed(t, resp), "anonymous"; got != want {
			t.Errorf("after signing out\n got: %s\nwant: %s", got, want)
		}

		if got, want := logOf(t, v), "login(alice) signout(alice)"; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
	})
}

// TestPageCacheStreamSession tests a sign-in on a page and a sign-out on App
// that clear the page cache and return neither a redirect nor a document.
// Their cache writes go out on an event stream, which sent its headers before
// the handler returned: the cookie of the session came too late, and an error
// was written into the stream instead of answering with its status.
func TestPageCacheStreamSession(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)
		login := v.Load(t, "/login/")

		resp := login.Do(t, http.MethodPost, "/login/submit-cached/",
			map[string]string{"user": ""})
		require.Equal(t, http.StatusBadRequest, resp.Status)

		resp = login.Do(t, http.MethodPost, "/login/submit-cached/",
			map[string]string{"user": "alice"})
		require.Equal(t, http.StatusOK, resp.Status, resp.Body)
		require.NotNil(t, setCookie(resp), "signing in set no session cookie")
		index := v.Get(t, "/")
		require.True(t, strings.HasPrefix(echoed(t, index), "user=alice "), index.Body)

		// The login page was rendered for a guest and carries no CSRF token.
		// A page loaded after the sign-in does.
		resp = v.Load(t, "/").Do(t, http.MethodPost, "/sign-out-cached/", nil)
		require.Equal(t, http.StatusOK, resp.Status, resp.Body)
		require.Equal(t, "anonymous", echoed(t, v.Get(t, "/")))
	})
}

// TestSecondSignInClosesTheFirstSession tests signing in again on a cookie
// that still names a live session.
//
// The response overwrites the cookie. A record left behind stays valid for
// whoever holds the old token, and with a zero ExpiresAt nothing collects it.
func TestSecondSignInClosesTheFirstSession(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)
		v := srv.Visitor(t)

		signIn(t, v, "alice", "Al")
		first := sessionToken(t, v)

		// The second sign-in carries the CSRF token of alice's session,
		// which an action of a signed-in visitor needs.
		signIn(t, v, "bob", "Bo")
		require.NotEqual(t, first, sessionToken(t, v), "the token was reused")

		_, _, ok, err := srv.sessions.ReadSessionFromCookie(first)
		require.NoError(t, err)
		require.False(t, ok, "alice's session outlived the sign-in that replaced it")

		v.SetSessionToken(first)
		resp := v.Get(t, "/")
		require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
		require.Equal(t, "anonymous", echoed(t, resp),
			"the replaced token still names a session")
	})
}

// TestSignOutOnGET tests a page GET that closes the session, which is what a
// sign-out reached by a plain navigation rather than by an action does.
func TestSignOutOnGET(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)
		signIn(t, v, "dave", "")

		resp := v.Get(t, "/sign-out-link/")
		require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
		require.Equal(t, "bye dave", echoed(t, resp))
		require.Contains(t, resp.Body, "<title>anonymous</title>", "%s", resp.Body)
		require.Nil(t, v.Cookie(cookieName), "the session cookie outlived the session")

		resp = v.Get(t, "/")
		require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
		require.Equal(t, "anonymous", echoed(t, resp))

		require.Equal(t, "login(dave) signoutlink(dave)", logOf(t, v))
	})
}

// TestSessionToken tests the handler parameter that asks for the token
// instead of the session.
func TestSessionToken(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		if resp := v.Get(t, "/token/"); resp.Status != http.StatusOK {
			t.Fatalf("status = %d, want 200\n%s", resp.Status, resp.Body)
		} else if got, want := echoed(t, resp), "no token"; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}

		signIn(t, v, "bob", "Bo")

		resp := v.Get(t, "/token/")
		if resp.Status != http.StatusOK {
			t.Fatalf("status = %d, want 200\n%s", resp.Status, resp.Body)
		}
		if got := echoed(t, resp); !strings.HasPrefix(got, "token of length ") {
			t.Errorf("the handler received no token: %s", got)
		}
	})
}

// TestStaleCookie tests a cookie the session manager does not know.
// The visitor continues as anonymous and the cookie is cleared,
// rather than being refused on every later request.
func TestStaleCookie(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)
		v.SetSessionToken("not-a-real-token")

		resp := v.Get(t, "/")
		if resp.Status != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.Status)
		}
		if got, want := echoed(t, resp), "anonymous"; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
		if ck := setCookie(resp); ck == nil || ck.Value != "" {
			t.Error("the stale cookie was left in place")
		}
	})
}

// TestErrorSentinel tests the status an action or
// page takes from the sentinel it returns.
func TestErrorSentinel(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)

		if status := v.Get(t, "/secret/").Status; status != http.StatusForbidden {
			t.Errorf("anonymous: status = %d, want %d", status, http.StatusForbidden)
		}

		resp := v.Load(t, "/login/").Do(t, http.MethodPost, "/login/submit/",
			map[string]string{"user": ""})
		if resp.Status != http.StatusBadRequest {
			t.Errorf("empty user: status = %d, want %d\n%s",
				resp.Status, http.StatusBadRequest, resp.Body)
		}

		signIn(t, v, "carol", "")
		if resp := v.Get(t, "/secret/"); resp.Status != http.StatusOK {
			t.Errorf("signed in: status = %d, want 200\n%s", resp.Status, resp.Body)
		} else if got, want := echoed(t, resp), "secret for carol"; got != want {
			t.Errorf(" got: %s\nwant: %s", got, want)
		}
	})
}

// TestActionHeadRendersFromTheSession tests the application-wide head of an
// action that answers with a document.
//
// The head of this app takes the session. An action that declares no session
// parameter renders it from the session the visitor sent, and a sign-in renders
// it from the session the same request created.
func TestActionHeadRendersFromTheSession(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)

		t.Run("guest", func(t *testing.T) {
			resp := srv.Visitor(t).Load(t, "/").Do(t, http.MethodPost, "/render/", nil)
			require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
			require.Contains(t, resp.Body, "<title>anonymous</title>", "%s", resp.Body)
		})

		t.Run("signed in", func(t *testing.T) {
			v := srv.Visitor(t)
			signIn(t, v, "alice", "")
			resp := v.Load(t, "/").Do(t, http.MethodPost, "/render/", nil)
			require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
			require.Contains(t, resp.Body, "<title>alice</title>", "%s", resp.Body)
		})

		t.Run("signed in by the same request", func(t *testing.T) {
			resp := srv.Visitor(t).Load(t, "/login/").Do(t, http.MethodPost,
				"/login/submit-inline/", map[string]string{"user": "bob"})
			require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
			require.Contains(t, resp.Body, "<title>bob</title>", "%s", resp.Body)
		})
	})
}

// TestError404HeadRendersFromTheSession tests the application-wide head of the 404 page,
// whose GET declares no session parameter.
func TestError404HeadRendersFromTheSession(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		v := serve(t, broker).Visitor(t)
		signIn(t, v, "carol", "")

		resp := v.Get(t, "/no-such-page-a1b2c3/")
		require.Equal(t, http.StatusNotFound, resp.Status, "%s", resp.Body)
		require.Contains(t, resp.Body, "<title>carol</title>", "%s", resp.Body)
	})
}

// TestInlineSignInCarriesCSRFScript tests a sign-in answering with a document
// instead of a navigation. Its Set-Cookie makes the CSRF token mandatory for
// every later action of the page, which sends what the script carries.
func TestInlineSignInCarriesCSRFScript(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		login := serve(t, broker).Visitor(t).Load(t, "/login/")

		resp := login.Do(t, http.MethodPost, "/login/submit-inline/",
			map[string]string{"user": "alice"})
		require.Equal(t, http.StatusOK, resp.Status, "%s", resp.Body)
		require.NotNil(t, setCookie(resp), "the response signed nobody in")
		require.Contains(t, resp.Body, "X-CSRF-Token",
			"the document carries no CSRF script:\n%s", resp.Body)

		// The token the script carries is what a later action sends.
		login.Do(t, http.MethodPost, "/login/rename/",
			map[string]string{"nickname": "al"}).WantStatus(t, http.StatusOK)
	})
}

// TestCSRF tests the token a state-changing request must carry once the
// visitor has a session.
// Without a session there is nothing to forge and an anonymous request is let through.
func TestCSRF(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)

		t.Run("anonymous request needs no token", func(t *testing.T) {
			resp := srv.Visitor(t).Load(t, "/login/").Do(t, http.MethodPost,
				"/login/submit/", map[string]string{"user": "dan"})
			if resp.Status != http.StatusOK {
				t.Errorf("status = %d, want 200\n%s", resp.Status, resp.Body)
			}
		})

		t.Run("session request without a token is refused", func(t *testing.T) {
			v := srv.Visitor(t)
			signIn(t, v, "erin", "")
			req := renameRequest(t, v, "E")
			req.Header.Del("X-CSRF-Token")
			if status := v.Send(t, req).Status; status != http.StatusForbidden {
				t.Errorf("status = %d, want %d", status, http.StatusForbidden)
			}
		})

		t.Run("session request with a wrong token is refused", func(t *testing.T) {
			v := srv.Visitor(t)
			signIn(t, v, "frank", "")
			req := renameRequest(t, v, "F")
			req.Header.Set("X-CSRF-Token", csrfToken(t, "the-token-of-someone-else"))
			if status := v.Send(t, req).Status; status != http.StatusForbidden {
				t.Errorf("status = %d, want %d", status, http.StatusForbidden)
			}
		})

		t.Run("session request with its token is served", func(t *testing.T) {
			v := srv.Visitor(t)
			signIn(t, v, "gina", "")
			if resp := v.Send(t, renameRequest(t, v, "G")); resp.Status != http.StatusOK {
				t.Errorf("status = %d, want 200\n%s", resp.Status, resp.Body)
			}
		})

		t.Run("a page load needs no token", func(t *testing.T) {
			v := srv.Visitor(t)
			signIn(t, v, "hank", "")
			if status := v.Get(t, "/").Status; status != http.StatusOK {
				t.Errorf("status = %d, want 200", status)
			}
		})
	})
}

// TestPrivateEvent tests an event addressed to a user. Of 2 signed-in visitors
// with a stream each, the user it names sees it and the other does not.
// A later notice to the other user is what shows the first one did not reach them.
func TestPrivateEvent(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)

		alice := srv.Visitor(t)
		signIn(t, alice, "alice", "")
		bob := srv.Visitor(t)
		signIn(t, bob, "bob", "")

		as := alice.Load(t, "/").Connect(t, nil)
		bs := bob.Load(t, "/").Connect(t, nil)

		login := alice.Load(t, "/login/")
		login.Do(t, http.MethodPost, "/login/notify/", notify("alice", "for alice")).
			WantStatus(t, http.StatusOK)
		as.WaitFor(t, datapagestest.TextContains("notice", "alice: for alice"))

		login.Do(t, http.MethodPost, "/login/notify/", notify("bob", "for bob")).
			WantStatus(t, http.StatusOK)
		bs.Absent(t,
			datapagestest.TextContains("notice", "for alice"),
			datapagestest.TextContains("notice", "bob: for bob"))
	})
}

// TestPrivateSignalScopedEvent tests that private and signal scopes combine
// without cross-user delivery, cross-signal delivery or duplicates.
func TestPrivateSignalScopedEvent(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)

		alice := srv.Visitor(t)
		signIn(t, alice, "alice", "")
		bob := srv.Visitor(t)
		signIn(t, bob, "bob", "")

		room := alice.Load(t, "/room/")
		aliceC1 := room.Connect(t, map[string]string{"calc_id": "c1"})
		aliceC2 := alice.Load(t, "/room/").Connect(t, map[string]string{"calc_id": "c2"})
		bobC1 := bob.Load(t, "/room/").Connect(t, map[string]string{"calc_id": "c1"})

		room.Do(t, http.MethodPost, "/room/update/",
			roomUpdate("alice", "c1", "for alice on c1")).WantStatus(t, http.StatusOK)
		aliceC1.WaitFor(t, datapagestest.TextContains("update", "alice: for alice on c1"))

		// The anchor of each stream is a later update addressed to its user and its
		// calc_id. The update for alice on c1 would come before it, and so would a
		// second copy of that update on alice's stream on c1.
		forAliceOnC1 := datapagestest.TextContains("update", "for alice on c1")
		room.Do(t, http.MethodPost, "/room/update/",
			roomUpdate("bob", "c1", "for bob on c1")).WantStatus(t, http.StatusOK)
		bobC1.Absent(t, forAliceOnC1,
			datapagestest.TextContains("update", "bob: for bob on c1"))
		room.Do(t, http.MethodPost, "/room/update/",
			roomUpdate("alice", "c2", "for alice on c2")).WantStatus(t, http.StatusOK)
		aliceC2.Absent(t, forAliceOnC1,
			datapagestest.TextContains("update", "alice: for alice on c2"))
		room.Do(t, http.MethodPost, "/room/update/",
			roomUpdate("alice", "c1", "again on c1")).WantStatus(t, http.StatusOK)
		aliceC1.Absent(t, forAliceOnC1,
			datapagestest.TextContains("update", "alice: again on c1"))
	})
}

// TestStreamEndsAtSessionExpiry tests a stream opened shortly before its
// session expires. From ExpiresAt on, a request carrying the session is served
// as a guest, and the stream ends then instead of rendering the user's private
// events until the tab closes. With the inmem broker, the session expires on the
// clock of a synctest bubble.
func TestStreamEndsAtSessionExpiry(t *testing.T) {
	t.Parallel()
	brokers.EachInBubble(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)
		alice := srv.Visitor(t)

		// Long enough to open the stream and see 1 event through it.
		expiresAt := time.Now().Add(2 * time.Second)
		token, err := srv.sessions.CreateSession(t.Context(),
			sessions.Record[app.SessionData]{UserID: "alice", ExpiresAt: expiresAt})
		require.NoError(t, err)
		alice.SetSessionToken(token)

		st := alice.Load(t, "/").Connect(t, nil)
		alice.Load(t, "/login/").Do(t, http.MethodPost, "/login/notify/",
			notify("alice", "before expiry")).WantStatus(t, http.StatusOK)
		st.WaitFor(t, datapagestest.TextContains("notice", "alice: before expiry"),
			"the stream received nothing before its session expired")
		require.True(t, time.Now().Before(expiresAt),
			"the session expired before the test checked the open stream")

		st.WaitFor(t, datapagestest.Reload(),
			"the page was not told to reload after its session expired")
		st.WaitEnd(t, "the stream is still open after its session expired")
		require.False(t, time.Now().Before(expiresAt),
			"the stream ended before its session expired")
	})
}

// TestStreamReloadsAtSignOut tests a stream whose session an action closes.
// The stream ends and tells the page to reload, which renders it for the guest
// instead of leaving the signed-out user's page in place.
func TestStreamReloadsAtSignOut(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		alice := serve(t, broker).Visitor(t)
		signIn(t, alice, "alice", "Al")
		index := alice.Load(t, "/")
		st := index.Connect(t, nil)

		index.Do(t, http.MethodPost, "/sign-out/", nil).WantStatus(t, http.StatusOK)

		st.WaitFor(t, datapagestest.Reload(),
			"the page was not told to reload after its session closed")
		st.WaitEnd(t, "the stream is still open after its session closed")
	})
}

// TestStreamReloadsAfterMissedSessionEnd tests a private stream that reconnects
// after its session ended while it was disconnected, as after a laptop sleeps.
// The stream tells the page to reload instead of refusing the guest. The page
// retries a refused stream without a limit and keeps showing the old session.
func TestStreamReloadsAfterMissedSessionEnd(t *testing.T) {
	t.Parallel()
	// Each creates a live session and returns its token and a function that ends it.
	live := map[string]func(t *testing.T, srv server) (token string, end func()){
		"closed": func(t *testing.T, srv server) (string, func()) {
			token, err := srv.sessions.CreateSession(t.Context(),
				sessions.Record[app.SessionData]{UserID: "alice"})
			require.NoError(t, err)
			return token, func() {
				require.NoError(t, srv.sessions.CloseSession(t.Context(), token))
			}
		},
		"expired": func(t *testing.T, srv server) (string, func()) {
			expiresAt := time.Now().Add(500 * time.Millisecond)
			token, err := srv.sessions.CreateSession(t.Context(),
				sessions.Record[app.SessionData]{UserID: "alice", ExpiresAt: expiresAt})
			require.NoError(t, err)
			return token, func() { time.Sleep(time.Until(expiresAt)) }
		},
	}

	for name, begin := range live {
		t.Run(name, func(t *testing.T) {
			brokers.EachInBubble(t, func(t *testing.T, broker messaging.Broker) {
				srv := serve(t, broker)
				alice := srv.Visitor(t)
				token, end := begin(t, srv)
				alice.SetSessionToken(token)

				// The page is rendered for the session, and the session ends before
				// the stream connects. /room/ has no anonymous stream a guest could be
				// sent to.
				room := alice.Load(t, "/room/")
				end()
				st := room.Connect(t, map[string]string{"calc_id": "c1"})

				ev := st.WaitFor(t, datapagestest.Reload(),
					"the page was not told to reload after its session ended")
				require.Equal(t, 5*time.Second, ev.Retry,
					"the page can reconnect before it reloads")
				st.WaitEnd(t, "the stream is still open without a session")
			})
		})
	}
}

// TestSignInAcceptsAnyUserID tests a user ID that cannot stand in a subject as it is.
// The ID names the subject every event addressed to that user is published to and
// subscribed by, and both sides escape it the same way.
//
// The wildcards are the cases that matter. Escaped, one is a literal token,
// which is what keeps a client signing in as "*" or ">" subscribed to itself
// and to nobody else.
func TestSignInAcceptsAnyUserID(t *testing.T) {
	t.Parallel()
	users := map[string]string{
		"email":     "alice@example.com",
		"separator": "a.b",
		"star":      "*",
		"gt":        ">",
		"space":     "a b",
		"percent":   "100%",
	}

	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		for name, user := range users {
			t.Run(name, func(t *testing.T) {
				srv := serve(t, broker)

				v := srv.Visitor(t)
				signIn(t, v, user, "")
				require.NotNil(t, v.Cookie(cookieName), "sign-in left no session cookie")

				bob := srv.Visitor(t)
				signIn(t, bob, "bob", "")

				s := v.Load(t, "/").Connect(t, nil)
				bs := bob.Load(t, "/").Connect(t, nil)

				// Addressed to bob. A user ID carrying a wildcard would
				// subscribe to every user, this one to nobody but itself.
				bob.Load(t, "/login/").Do(t, http.MethodPost, "/login/notify/",
					notify("bob", "for bob")).WantStatus(t, http.StatusOK)
				bs.WaitFor(t, datapagestest.TextContains("notice", "bob: for bob"))

				// Addressed to the user itself, which is what proves the
				// publish and the subscription agree on the escaped subject.
				// A notice to bob that reached this stream would come before it.
				v.Load(t, "/login/").Do(t, http.MethodPost, "/login/notify/",
					notify(user, "for me")).WantStatus(t, http.StatusOK)
				s.Absent(t,
					datapagestest.TextContains("notice", "for bob"),
					datapagestest.TextContains("notice", user+": for me"))
			})
		}
	})
}

// TestStreamOpensForAnyUserID tests a session whose user ID cannot stand in
// a subject as it is, created straight in the store the way a manager of an
// application's own would. The stream subscribes by the escaped ID,
// so it opens for one a sign-in never went through.
func TestStreamOpensForAnyUserID(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		for name, userID := range map[string]string{
			"star":      "*",
			"gt":        ">",
			"separator": "a.b",
			"email":     "alice@example.com",
		} {
			t.Run(name, func(t *testing.T) {
				srv := serve(t, broker)

				token, err := srv.sessions.CreateSession(t.Context(),
					sessions.Record[app.SessionData]{
						UserID:    userID,
						IssuedAt:  time.Now(),
						ExpiresAt: time.Now().Add(time.Hour),
					})
				require.NoError(t, err, "creating the session")

				v := srv.Visitor(t)
				v.SetSessionToken(token)

				// A session the stream reads as a guest's is redirected to the
				// anonymous stream.
				st := v.Load(t, "/").Connect(t, nil)
				require.Equal(t, "/_$/", st.Path(),
					"the stream refused user ID %q", userID)
			})
		}
	})
}

// TestAnonymousStream tests the stream a page serves to a visitor with no session,
// on a page that handles a public event and a private one.
//
// The server generates a second stream handler for that page.
// An anonymous connection is served by it and subscribes to the public event alone.
// The private event is addressed to a user, and a connection with no user must
// never be given it: that is one visitor reading another's messages.
func TestAnonymousStream(t *testing.T) {
	t.Parallel()
	brokers.Each(t, func(t *testing.T, broker messaging.Broker) {
		srv := serve(t, broker)

		anon := srv.Visitor(t)
		anonStream := anon.Load(t, "/").Connect(t, nil)

		alice := srv.Visitor(t)
		signIn(t, alice, "alice", "")
		aliceStream := alice.Load(t, "/").Connect(t, nil)

		// The 2 connections are served by 2 different handlers.
		require.Equal(t, "/_$/anon/", anonStream.Path())
		require.Equal(t, "/_$/", aliceStream.Path())

		// The public event reaches both.
		login := alice.Load(t, "/login/")
		login.Do(t, http.MethodPost, "/login/broadcast/",
			map[string]string{"text": "for everyone"}).WantStatus(t, http.StatusOK)
		anonStream.WaitFor(t, datapagestest.TextContains("broadcast", "for everyone"))
		aliceStream.WaitFor(t, datapagestest.TextContains("broadcast", "for everyone"))

		// The private one reaches only the user it names. A later public event is
		// what shows it did not reach the anonymous stream.
		login.Do(t, http.MethodPost, "/login/notify/", notify("alice", "for alice")).
			WantStatus(t, http.StatusOK)
		aliceStream.WaitFor(t, datapagestest.TextContains("notice", "alice: for alice"))
		login.Do(t, http.MethodPost, "/login/broadcast/",
			map[string]string{"text": "after"}).WantStatus(t, http.StatusOK)
		anonStream.Absent(t,
			datapagestest.TextContains("notice", "for alice"),
			datapagestest.TextContains("broadcast", "after"))
	})
}

// logOf reads what the application recorded while its handlers ran.
func logOf(t *testing.T, v *datapagestest.Visitor) string {
	t.Helper()
	return echoed(t, v.Get(t, "/log/"))
}
