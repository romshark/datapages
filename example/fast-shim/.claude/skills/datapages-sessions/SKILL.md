---
name: datapages-sessions
description: >-
  Add Datapages authentication: define and read the Session type, open and
  close sessions, configure CSRF protection and choose a session manager. Use
  when adding sign-in, sign-out or authenticated handlers to a Datapages app.
---

# Sessions

Read `datapages` first for the build loop, hard rules and naming conventions.

Declare the session data and alias once in the app package. Do not declare them when the app has no authentication.

```go
type SessionData struct{ Name string }

type Session = datapages.Session[SessionData]
```

Use `struct{}` when the session has no application data. Every handler must use the same data type. Use the shared alias in every handler.

## Read

Pages, actions, event handlers and stream hooks may take `session Session`. It is read-only and provides `UserID()`, `IsGuest()`, `Token()`, `IssuedAt()`, `ExpiresAt()` and `Data()`. Datapages treats an expired session as unauthenticated and deletes its cookie.

Add `session Session` to every action that must reject a closed or expired session. Without this parameter, Datapages checks only that the request has a session cookie. It does not read the session store.

## Open and close

Return values, from a `GET` or an action:

```go
// POSTSignIn is /sign-in
func (*App) POSTSignIn(
	r *http.Request,
	signals datapages.Signals[struct {
		Name string `json:"name"`
	}],
) (
	newSession datapages.NewSession[SessionData],
	redirect datapages.Redirect,
	err error,
) {
	name := signals.Values.Name
	if err := datapages.ValidateUserID(name); err != nil {
		return datapages.NewSession[SessionData]{},
			datapages.Redirect{}, datapages.ErrBadRequest
	}
	return datapages.NewSession[SessionData]{
		UserID: name,
		Data:   SessionData{Name: name},
	}, datapages.Redirect{URL: href.PageIndex()}, nil
}
```

`NewSession` contains `UserID`, `Data` and an optional `ExpiresAt`. Datapages creates the token and sets the issue time. A zero `UserID` creates no session. To sign out, return `closeSession datapages.CloseSession` set to `true`.

`ExpiresAt` also sets the cookie's `Max-Age` and `Expires` attributes. A nonzero value can keep the client signed in after a browser restart. A zero value creates a cookie that the browser deletes when it closes. The session record remains in the store until the application deletes it.

Opening or closing a session cannot be combined with a `datapages.SSE` parameter. The SSE response headers are sent before the handler runs, so the handler cannot change the cookie. Sign in or out without `sse`, then return a `redirect`.

With an offline page cache, sign-in and sign-out actions also take `pageCache`
and call `ClearAll()`. A snapshot cached for a guest still shows signed-out
navigation after login. See `datapages-offline`.

## CSRF

CSRF protection is enabled for every app with a session type. Datapages derives the token from the session. Do not set a CSRF header in a template. A normal browser form submission does not include the token. Submit forms through a Datastar action as shown in `datapages-templates`. Use `datapages.WithCSRFProtection(datapages.CSRFConfig{...})` only to replace the token source or disable protection.

## Manager

The store is a server option, see `datapages-server`:

```go
opts = append(opts, datapages.WithSessionManager[app.SessionData](mgr))
```

Specify the data type in this call. Go cannot infer it here. The type argument also makes the compiler check that the manager matches the app's session type.

Use `modules/sessions/natskv`. Use `modules/sessions/inmem` only for development. It stores sessions in memory and loses them on restart.

Datapages does not scan the store for expired records. It deletes an expired record only when a client sends that session. Call `mgr.DeleteExpired(ctx)` on a ticker to delete all other expired records.
