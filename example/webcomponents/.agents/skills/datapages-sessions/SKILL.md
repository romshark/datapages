---
name: datapages-sessions
description: >-
  Datapages authentication: the Session type, reading it in handlers,
  opening and closing sessions, CSRF protection and choosing a session manager.
  Activate when adding sign-in, sign-out or authenticated handlers to a
  Datapages app.
---

# Sessions

Read `datapages` first for the build loop, hard rules and naming conventions.

Declare the payload and the alias once in the app package. Skip all of this if the app needs no authentication.

```go
type SessionData struct{ Name string }

type Session = datapages.Session[SessionData]
```

Use `struct{}` when there is no payload. Every handler must use the same `Data` type, which is why the alias is declared once and used everywhere.

## Read

Take `session Session` in any page, action, event handler or stream hook. It is read-only: `UserID()`, `IsGuest()`, `Token()`, `IssuedAt()`, `ExpiresAt()`, `Data()`. An expired client counts as unauthenticated and loses its cookie.

Declare it in every action that must not run for a stale session: an action without it is checked against the session cookie alone and never reads the store, so the cookie of a closed or expired session passes.

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

`NewSession` carries `UserID`, `Data` and an optional `ExpiresAt`; Datapages mints the token and stamps the issue time. A zero `UserID` creates nothing. Sign out by returning `closeSession datapages.CloseSession` as `true`.

`ExpiresAt` also becomes the cookie's `Max-Age` and `Expires`, which keeps the client signed in across browser restarts. A zero `ExpiresAt` writes a cookie the browser drops when it closes. The record stays in the store until the application removes it.

Neither works next to a `datapages.SSE` parameter: the headers the cookie travels in are already out. Sign in or out without `sse` and use `redirect`.

With an offline page cache, both also take `pageCache` and call `ClearAll()`: a snapshot cached for a guest still shows the signed-out navigation after login. See `datapages-offline`.

## CSRF

CSRF is enabled for every app with a session type. Datapages derives the token from the session; no option is needed. Do not set a CSRF header in a template. Browser form submissions do not carry the token. Submit forms through a Datastar action as shown in `datapages-templates`. Use `datapages.WithCSRFProtection(datapages.CSRFConfig{...})` only to replace the token source or disable protection.

## Manager

The store is a server option, see `datapages-server`:

```go
opts = append(opts, datapages.WithSessionManager[app.SessionData](mgr))
```

Name the data type at the call: it is not inferred, and naming it is what makes the compiler check the manager against what the app declares.

Use `modules/sessions/natskv`. `modules/sessions/inmem` is for a single instance that may lose its sessions on restart.

The framework never collects expired records: reading a session only reclaims the ones a client returns to. Call `mgr.DeleteExpired(ctx)` on a ticker.
