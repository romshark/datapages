# Sessions and auth

A session cookie identifies the visitor of a request. Handlers can read typed session data, create or close a session, and reject guests. A session manager stores the records behind the cookies.

## Declare the session type

Declare the data the application keeps in a session and an alias for the session type in the app package:

```go title="app/app.go"
// SessionData is what the application keeps in a session.
type SessionData struct {
	Name string
}

type Session = datapages.Session[SessionData]
```

Use `struct{}` as the data type when the application keeps nothing beyond the user ID, as the scaffold does. Every handler must use the same data type, and the alias keeps the signatures short. The second type argument of `datapages.NewServer` names the data type, and a session manager option passes the store:

```go title="cmd/server/main.go"
opts = append(opts, datapages.WithSessionManager[app.SessionData](sessionManager))
```

Write the type argument of `WithSessionManager` out. The compiler then checks that the manager stores that type, which `NewServer` otherwise checks only at startup and reports as an error. An application without sign-in declares no session type and passes `datapages.DisableSessions` to `NewServer`.

## Read the session

A page `GET`, an action, an event handler, a stream hook, `App.Head` and a `GETXXX` file action may take `session Session`:

```go title="app/app.go"
func (PageIndex) GET(r *http.Request, session Session) (
	body datapages.Component, err error,
) {
	if session.IsGuest() {
		return pageIndex("", time.Time{}), nil
	}
	return pageIndex(session.Data().Name, session.ExpiresAt()), nil
}
```

The session is read-only and provides `UserID()`, `IsGuest()`, `Token()`, `IssuedAt()`, `ExpiresAt()` and `Data()`. A visitor without a valid session gets the zero value, for which `IsGuest()` reports true. A page rendered for a session cookie goes out with `Cache-Control: private`.

An action without a session parameter does not read the store. Guests can call it. A request with the cookie of a closed or expired session also reaches it when the CSRF token matches. Add `session Session` to every action that must reject these requests. Check `IsGuest()` when only signed-in visitors may call the action.

## Sign in

An action or a page `GET` signs a visitor in by returning a `datapages.NewSession`:

```go title="app/app.go"
// POSTSubmit is /login/submit
func (p PageLogin) POSTSubmit(
	r *http.Request,
	signals datapages.Signals[struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}],
) (
	newSession datapages.NewSession[SessionData],
	redirect datapages.Redirect,
	err error,
) {
	user, err := p.App.Users.Check(r.Context(), signals.Values.Email, signals.Values.Password)
	if errors.Is(err, users.ErrBadCredentials) {
		return newSession, redirect, datapages.ErrForbidden
	}
	if err != nil {
		return newSession, redirect, err
	}
	return datapages.NewSession[SessionData]{
		UserID:    user.ID,
		ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
		Data:      SessionData{Name: user.Name},
	}, datapages.Redirect{URL: href.PageIndex()}, nil
}
```

```templ title="app/app.templ"
templ pageLogin() {
	<form
		data-signals:email="''"
		data-signals:password="''"
		data-on:submit__prevent={ action.PageLogin.Submit.POST() }
	>
		<input type="email" data-bind="email" required/>
		<input type="password" data-bind="password" required/>
		<button type="submit">Sign in</button>
	</form>
}
```

Datapages generates the session token, sets the issue time, stores the record through the session manager and sets the cookie. It first closes the session the request arrived with, which ends the streams of that session. For a Datastar request, the redirect is a `text/javascript` response that assigns `window.location`.

A `NewSession` with an empty `UserID` creates no session. A user ID longer than `datapages.MaxUserIDEncodedLen` bytes once escaped fails the request with 500. When the ID comes from input, check it with `datapages.ValidateUserID` first and return `datapages.ErrBadRequest`.

An action that takes `datapages.SSE` cannot open or close a session, and `datapages gen` rejects the combination. The SSE response sends its headers before the handler runs, and the cookie travels in those headers. Sign in without `sse` and return a `redirect`.

## Sign out

Return `datapages.CloseSession` set to `true`:

```go title="app/app.go"
// POSTSignOut is /sign-out
func (*App) POSTSignOut(r *http.Request) (
	closeSession datapages.CloseSession,
	redirect datapages.Redirect,
	err error,
) {
	return true, datapages.Redirect{URL: href.PageLogin()}, nil
}
```

Datapages closes the session in the store and deletes the cookie. The action needs a CSRF token like every other one, which the page sends when it was rendered for the session.

## Expiry and cookie lifetime

`NewSession.ExpiresAt` sets the `Max-Age` and `Expires` attributes of the cookie, which keeps the visitor signed in across browser restarts. A zero `ExpiresAt` writes a cookie that the browser deletes when it closes, and a session that never expires on the server.

From `ExpiresAt` on, Datapages treats a request with the session as a guest, deletes the cookie and deletes the record. It deletes only the records of sessions that a client sends again. Delete the other expired records on a schedule:

```go title="cmd/server/main.go"
func deleteExpiredSessions(ctx context.Context, m sessions.ExpiredDeleter) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := m.DeleteExpired(ctx); err != nil {
				slog.Error("deleting expired sessions", slog.Any("err", err))
			}
		}
	}
}
```

Start it with `go deleteExpiredSessions(ctx, sessionManager)` in `main`. The cookie is named `sessiontoken` and is `HttpOnly`, `Secure` and `SameSite=Lax`. `datapages.WithSessions` changes its name and domain and can drop `Secure` for a deployment that runs on plain HTTP.

## Open pages when a session ends

A stream whose page reads the session ends when the session closes or reaches `ExpiresAt`. A page reads the session in its stream when it handles a user-addressed event or when an event handler or stream hook takes `session`. 1 second after the stream ends, the page reloads and renders for the session the browser holds, if any. The 1 second delay lets the tab that signed out follow its own redirect first.

Other pages keep what they rendered until they reload. A page rendered for a guest carries no CSRF token, so after a sign-in in another tab of the same browser its state-changing actions receive 403 until it reloads.

## CSRF protection

CSRF protection is on for every application with a session type. Datapages derives the token from the session token and masks it with fresh random bytes on every render. It writes a script into each page rendered for a session cookie, which adds the token in the `X-CSRF-Token` header to each Datastar request to the same origin, except `GET`, `HEAD`, `OPTIONS` and `QUERY`. Templates set nothing.

The server checks the `POST`, `PUT`, `PATCH` and `DELETE` actions of every request that carries a session cookie and answers 403 to a missing or wrong token. A request without a session cookie passes. A plain HTML form submission carries no token, so a form of a signed-in visitor receives 403. Submit forms through Datastar actions, as the login form above does and [Templates and forms](templates.md#forms) shows. A `QUERYXXX` action skips the check and must not change state.

`datapages.WithCSRFProtection(datapages.CSRFConfig{...})` replaces the token source through `Tokens` or turns the protection off through `Disabled`. Turn it off only when something in front of the server rejects cross-origin requests.

## Choose a session manager

| Module | Use |
| ------ | --- |
| `modules/sessions/inmem` | Development and tests. It keeps sessions in memory and loses them on restart. |
| `modules/sessions/natskv` | Production. It stores sessions in the NATS key-value bucket `SESSIONS` and puts the record key into the cookie, encrypted with AES-128-GCM, which keeps the user ID from the client. |
| Your own `sessions.Manager[Data]` | Any other store. |

The scaffold creates the NATS manager with `natskv.New[app.SessionData](conn, sessions.DefaultTokenGenerator{Length: sessions.DefaultTokenLen}, natskv.Config{EncryptionKey: sessionEncryptionKey})`. The key has 16 bytes. The scaffold reads it hex-encoded from `SESSION_ENCRYPTION_KEY`. `datapages init` writes a random value for it to `.env` and adds `.env` to `.gitignore`. `natskv.Config.PreviousEncryptionKeys` keeps earlier keys for reading existing cookies while the key rotates. The application stores and rotates the key, see [Security](security.md#manage-session-keys).

A store of your own implements the 5 methods of `sessions.Manager`, which are `ReadSessionFromCookie`, `CreateSession`, `CloseSession`, `NotifyClosed` and `DeleteExpired`. `ReadSessionFromCookie` returns the cookie value as the token, because the CSRF token derives from it. `NotifyClosed` reports a closed session, which is what ends its streams. [`example/sqlitesessions`](../example/sqlitesessions/) stores sessions in SQLite.

## Authorize in the application

A session says who the visitor is, never what the visitor may see or do. Check permissions in each handler and return `datapages.ErrForbidden`:

```go title="app/app.go"
// POSTDeleteNote is /notes/{id}/delete
func (p PageIndex) POSTDeleteNote(
	r *http.Request,
	session Session,
	path datapages.Path[struct {
		ID string `path:"id"`
	}],
) error {
	if session.IsGuest() {
		return datapages.ErrForbidden
	}
	note, err := p.App.Users.Note(r.Context(), path.Values.ID)
	if err != nil {
		return err
	}
	if note.AuthorID != session.UserID() {
		return datapages.ErrForbidden
	}
	return p.App.Users.DeleteNote(r.Context(), note.ID)
}
```

The same holds for a page `GET`, a `GETXXX` file and an event handler, which renders for the visitor of its tab. [Security](security.md#authorize-every-request) covers more cases.

## Clear the offline cache

An application that caches pages with `pageCache` keeps snapshots in the browser after a sign-out, readable by any script of the origin. Call `pageCache.ClearAll()` in the actions that sign in and sign out, as [Offline and shims](offline.md#clear-the-cache-at-sign-in-and-sign-out) shows.

## See also

- [`Session`](../SPECIFICATION.md#parameter-session-datapagessessiondata): the methods, expiry and the CSRF check without a session parameter.
- [`NewSession`](../SPECIFICATION.md#return-value-newsession-datapagesnewsessiondata), [`ValidateUserID`](../SPECIFICATION.md#validating-a-user-id) and [`CloseSession`](../SPECIFICATION.md#return-value-closesession-datapagesclosesession): signing in and out.
- [Plain Forms and CSRF](../SPECIFICATION.md#plain-forms-and-csrf): why authenticated forms use Datastar actions.
- [Events](events.md#address-a-user): events addressed to the streams of 1 user.
- [Security](security.md) and [SECURITY.md](../SECURITY.md): authorization, key management and what the deployment provides.
- [`example/sqlitesessions`](../example/sqlitesessions/): a session manager on SQLite.
